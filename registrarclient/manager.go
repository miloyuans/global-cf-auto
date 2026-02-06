package registrarclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"DomainC/config"
)

// Manager 提供基于配置的注册商查询/修改能力。
type Manager struct {
	client     Client
	registrars []config.Registrar
}

func NewManager(client Client, registrars []config.Registrar) *Manager {
	if client == nil {
		client = NewClient()
	}
	return &Manager{client: client, registrars: registrars}
}

func (m *Manager) Registrars() []config.Registrar {
	out := make([]config.Registrar, 0, len(m.registrars))
	for _, r := range m.registrars {
		if strings.TrimSpace(r.Label) == "" || strings.TrimSpace(r.Type) == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}

func (m *Manager) RegistrarByLabel(label string) (config.Registrar, bool) {
	if strings.TrimSpace(label) == "" {
		return config.Registrar{}, false
	}
	for _, r := range m.registrars {
		if strings.EqualFold(strings.TrimSpace(r.Label), strings.TrimSpace(label)) {
			return r, true
		}
	}
	return config.Registrar{}, false
}

func (m *Manager) registrarsForDomain(domain string) []config.Registrar {
	if strings.TrimSpace(domain) == "" {
		return nil
	}
	results := make([]config.Registrar, 0, len(m.registrars))
	for _, r := range m.registrars {
		if strings.TrimSpace(r.Label) == "" || strings.TrimSpace(r.Type) == "" {
			continue
		}
		results = append(results, r)
	}
	return results
}

// SetNameServersForDomain 尝试将 NS 写入到对应注册商。
func (m *Manager) SetNameServersForDomain(ctx context.Context, domain string, nameServers []string) (config.Registrar, error) {
	if len(m.registrars) == 0 {
		return config.Registrar{}, fmt.Errorf("未配置注册商")
	}

	var lastErr error
	for _, r := range m.registrarsForDomain(domain) {
		err := m.client.SetNameServers(ctx, r, domain, nameServers)
		if err != nil {
			if errors.Is(err, ErrDomainNotFound) {
				continue
			}
			lastErr = fmt.Errorf("[%s] %w", r.Label, err)
			continue
		}
		return r, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("未在任何注册商账号下找到该域名")
	}
	return config.Registrar{}, lastErr
}
func (m *Manager) GetExpireAtForDomain(ctx context.Context, domain string) (config.Registrar, time.Time, error) {
	if len(m.registrars) == 0 {
		return config.Registrar{}, time.Time{}, fmt.Errorf("未配置注册商")
	}
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return config.Registrar{}, time.Time{}, fmt.Errorf("域名不能为空")
	}

	// 通过 type assertion 使用底层实现，避免改动 Client 接口
	type namecheapExpireGetter interface {
		namecheapGetExpireAt(ctx context.Context, cfg config.NamecheapConfig, domain string) (time.Time, error)
	}

	getter, ok := m.client.(namecheapExpireGetter)
	if !ok {
		return config.Registrar{}, time.Time{}, fmt.Errorf("当前 client 不支持获取到期时间")
	}

	var lastErr error
	for _, r := range m.registrarsForDomain(domain) {
		if strings.ToLower(strings.TrimSpace(r.Type)) != "namecheap" || r.Namecheap == nil {
			continue
		}

		expAt, err := getter.namecheapGetExpireAt(ctx, *r.Namecheap, domain)
		if err != nil {
			// 该账号下没有这个域名：继续尝试下一个账号
			if errors.Is(err, ErrDomainNotFound) {
				continue
			}
			lastErr = fmt.Errorf("[%s] %w", r.Label, err)
			continue
		}

		return r, expAt, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("未在任何 namecheap 账号下找到该域名")
	}
	return config.Registrar{}, time.Time{}, lastErr
}

// GetNameServersForDomain 尝试从注册商读取 NS。
func (m *Manager) GetNameServersForDomain(ctx context.Context, domain string) (config.Registrar, []string, error) {
	if len(m.registrars) == 0 {
		return config.Registrar{}, nil, fmt.Errorf("未配置注册商")
	}
	var lastErr error
	for _, r := range m.registrarsForDomain(domain) {
		ns, err := m.client.GetNameServers(ctx, r, domain)
		if err != nil {
			lastErr = err
			continue
		}
		return r, ns, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("没有可用的注册商")
	}
	return config.Registrar{}, nil, lastErr
}

// ListDomainsForRegistrar 查询指定注册商的域名列表。
func (m *Manager) ListDomainsForRegistrar(ctx context.Context, registrar config.Registrar) ([]string, error) {
	return m.client.ListDomains(ctx, registrar)
}
