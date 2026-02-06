package registrarclient

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"DomainC/config"
)

// Client 定义注册商基础操作接口。
type Client interface {
	GetNameServers(ctx context.Context, registrar config.Registrar, domain string) ([]string, error)
	SetNameServers(ctx context.Context, registrar config.Registrar, domain string, nameServers []string) error
	ListDomains(ctx context.Context, registrar config.Registrar) ([]string, error)
}

type apiClient struct {
	httpClient *http.Client
}

// NewClient 返回默认注册商 API 客户端实现。
func NewClient() Client {
	return &apiClient{httpClient: &http.Client{Timeout: 15 * time.Second}}
}

var (
	ErrDomainNotFound       = errors.New("domain not found")
	ErrUnsupportedRegistrar = errors.New("unsupported registrar")
)

func (c *apiClient) GetNameServers(ctx context.Context, registrar config.Registrar, domain string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(registrar.Type)) {
	case "namecheap":
		if registrar.Namecheap == nil {
			return nil, fmt.Errorf("namecheap 配置缺失")
		}
		return c.namecheapGetNameServers(ctx, *registrar.Namecheap, domain)
	case "godaddy":
		if registrar.GoDaddy == nil {
			return nil, fmt.Errorf("godaddy 配置缺失")
		}
		return c.goDaddyGetNameServers(ctx, *registrar.GoDaddy, domain)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedRegistrar, registrar.Type)
	}
}

func (c *apiClient) SetNameServers(ctx context.Context, registrar config.Registrar, domain string, nameServers []string) error {
	switch strings.ToLower(strings.TrimSpace(registrar.Type)) {
	case "namecheap":
		if registrar.Namecheap == nil {
			return fmt.Errorf("namecheap 配置缺失")
		}
		return c.namecheapSetNameServers(ctx, *registrar.Namecheap, domain, nameServers)
	case "godaddy":
		if registrar.GoDaddy == nil {
			return fmt.Errorf("godaddy 配置缺失")
		}
		return c.goDaddySetNameServers(ctx, *registrar.GoDaddy, domain, nameServers)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedRegistrar, registrar.Type)
	}
}

func (c *apiClient) ListDomains(ctx context.Context, registrar config.Registrar) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(registrar.Type)) {
	case "namecheap":
		if registrar.Namecheap == nil {
			return nil, fmt.Errorf("namecheap 配置缺失")
		}
		return c.namecheapListDomains(ctx, *registrar.Namecheap)
	case "godaddy":
		if registrar.GoDaddy == nil {
			return nil, fmt.Errorf("godaddy 配置缺失")
		}
		return c.goDaddyListDomains(ctx, *registrar.GoDaddy)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedRegistrar, registrar.Type)
	}
}

type namecheapResponse struct {
	Status          string                   `xml:"Status,attr"`
	Errors          namecheapErrors          `xml:"Errors"`
	CommandResponse namecheapCommandResponse `xml:"CommandResponse"`
}

type namecheapErrors struct {
	Items []namecheapError `xml:"Error"`
}

type namecheapError struct {
	Number  string `xml:"Number,attr"`
	Message string `xml:",chardata"`
}
type namecheapDomainGetInfoResult struct {
	DomainName string `xml:"DomainName,attr"`
	Expires    string `xml:"Expires,attr"`
	IsExpired  string `xml:"IsExpired,attr"`
}
type namecheapCommandResponse struct {
	DomainDNSGetListResult   namecheapGetListResult       `xml:"DomainDNSGetListResult"`
	DomainDNSSetCustomResult namecheapSetCustomResult     `xml:"DomainDNSSetCustomResult"`
	DomainGetListResult      namecheapDomainListResult    `xml:"DomainGetListResult"`
	DomainGetInfoResult      namecheapDomainGetInfoResult `xml:"DomainGetInfoResult"`
}
type namecheapGetListResult struct {
	NameServers []string `xml:"Nameserver"`
}

type namecheapSetCustomResult struct {
	Updated string `xml:"Updated,attr"`
}

type namecheapDomainListResult struct {
	Domains []namecheapDomainListItem `xml:"Domain"`
}

type namecheapDomainListItem struct {
	Name string `xml:"Name,attr"`
}

func (c *apiClient) namecheapGetNameServers(ctx context.Context, cfg config.NamecheapConfig, domain string) ([]string, error) {
	sld, tld, err := splitDomain(domain)
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("ApiUser", cfg.User)
	params.Set("ApiKey", cfg.APIKey)
	params.Set("UserName", cfg.User)
	params.Set("ClientIp", cfg.ClientIP)
	params.Set("Command", "namecheap.domains.dns.getList")
	params.Set("SLD", sld)
	params.Set("TLD", tld)

	data, err := c.namecheapRequest(ctx, params)
	if err != nil {
		return nil, err
	}
	resp, err := parseNamecheapResponse(data)
	if err != nil {
		return nil, err
	}
	if len(resp.CommandResponse.DomainDNSGetListResult.NameServers) == 0 {
		return nil, fmt.Errorf("namecheap 未返回 NS")
	}
	return resp.CommandResponse.DomainDNSGetListResult.NameServers, nil
}
func (c *apiClient) namecheapGetExpireAt(ctx context.Context, cfg config.NamecheapConfig, domain string) (time.Time, error) {
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	if domain == "" {
		return time.Time{}, fmt.Errorf("域名不能为空")
	}

	params := url.Values{}
	params.Set("ApiUser", cfg.User)
	params.Set("ApiKey", cfg.APIKey)
	params.Set("UserName", cfg.User)
	params.Set("ClientIp", cfg.ClientIP)
	params.Set("Command", "namecheap.domains.getInfo")
	params.Set("DomainName", domain)

	data, err := c.namecheapRequest(ctx, params)
	if err != nil {
		return time.Time{}, err
	}
	resp, err := parseNamecheapResponse(data)
	if err != nil {
		return time.Time{}, err
	}

	exp := strings.TrimSpace(resp.CommandResponse.DomainGetInfoResult.Expires)
	if exp == "" {
		return time.Time{}, fmt.Errorf("namecheap 未返回 Expires")
	}

	t, err := time.Parse(time.RFC3339, exp)
	if err != nil {
		return time.Time{}, fmt.Errorf("namecheap Expires 解析失败: %w", err)
	}
	return t, nil
}

func equalStringSliceIgnoreOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, x := range a {
		m[strings.ToLower(x)]++
	}
	for _, x := range b {
		k := strings.ToLower(x)
		if m[k] == 0 {
			return false
		}
		m[k]--
	}
	return true
}

func (c *apiClient) namecheapSetNameServers(ctx context.Context, cfg config.NamecheapConfig, domain string, nameServers []string) error {
	current, err := c.namecheapGetNameServers(ctx, cfg, domain)
	if err == nil {
		if equalStringSliceIgnoreOrder(current, nameServers) {
			return nil // 已经一致，直接返回
		}
	}
	if len(nameServers) == 0 {
		return fmt.Errorf("NS 不能为空")
	}
	sld, tld, err := splitDomain(domain)
	if err != nil {
		return err
	}
	params := url.Values{}
	params.Set("ApiUser", cfg.User)
	params.Set("ApiKey", cfg.APIKey)
	params.Set("UserName", cfg.User)
	params.Set("ClientIp", cfg.ClientIP)
	params.Set("Command", "namecheap.domains.dns.setCustom")
	params.Set("SLD", sld)
	params.Set("TLD", tld)
	params.Set("Nameservers", strings.Join(nameServers, ","))

	data, err := c.namecheapRequest(ctx, params)
	if err != nil {
		return err
	}
	_, err = parseNamecheapResponse(data)
	return err
}

func (c *apiClient) namecheapListDomains(ctx context.Context, cfg config.NamecheapConfig) ([]string, error) {
	params := url.Values{}
	params.Set("ApiUser", cfg.User)
	params.Set("ApiKey", cfg.APIKey)
	params.Set("UserName", cfg.User)
	params.Set("ClientIp", cfg.ClientIP)
	params.Set("Command", "namecheap.domains.getList")
	params.Set("PageSize", "100")
	params.Set("Page", "1")

	data, err := c.namecheapRequest(ctx, params)
	if err != nil {
		return nil, err
	}
	resp, err := parseNamecheapResponse(data)
	if err != nil {
		return nil, err
	}
	domains := make([]string, 0, len(resp.CommandResponse.DomainGetListResult.Domains))
	for _, item := range resp.CommandResponse.DomainGetListResult.Domains {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		domains = append(domains, item.Name)
	}
	if len(domains) == 0 {
		return nil, fmt.Errorf("namecheap 未返回域名列表")
	}
	return domains, nil
}

func (c *apiClient) namecheapRequest(ctx context.Context, params url.Values) ([]byte, error) {
	endpoint := "https://api.namecheap.com/xml.response"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("namecheap 请求创建失败: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("namecheap 请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("namecheap 读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("namecheap 响应异常: %s", strings.TrimSpace(string(data)))
	}
	return data, nil
}

func parseNamecheapResponse(data []byte) (namecheapResponse, error) {
	var resp namecheapResponse
	if err := xml.Unmarshal(data, &resp); err != nil {
		return resp, fmt.Errorf("namecheap 解析失败: %w", err)
	}

	if strings.ToUpper(resp.Status) != "OK" {
		if len(resp.Errors.Items) == 0 {
			return resp, fmt.Errorf("namecheap 返回非 OK 状态")
		}
	}

	if len(resp.Errors.Items) > 0 {
		var messages []string
		for _, item := range resp.Errors.Items {
			msg := strings.TrimSpace(item.Message)
			if msg == "" {
				continue
			}
			messages = append(messages, msg)
			if strings.Contains(strings.ToLower(msg), "domain not found") {
				return resp, ErrDomainNotFound
			}
		}
		return resp, fmt.Errorf("namecheap 错误: %s", strings.Join(messages, "; "))
	}

	return resp, nil
}

type goDaddyDomain struct {
	NameServers []string `json:"nameServers"`
}

type goDaddyDomainListItem struct {
	Domain string `json:"domain"`
}

type goDaddyNameServer struct {
	Name string `json:"name"`
}

func (c *apiClient) goDaddyGetNameServers(ctx context.Context, cfg config.GoDaddyConfig, domain string) ([]string, error) {
	endpoint := fmt.Sprintf("https://api.godaddy.com/v1/domains/%s", domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("godaddy 请求创建失败: %w", err)
	}
	applyGoDaddyAuth(req, cfg)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("godaddy 请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("godaddy 读取响应失败: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrDomainNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("godaddy 响应异常: %s", strings.TrimSpace(string(data)))
	}
	var result goDaddyDomain
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("godaddy 解析失败: %w", err)
	}
	if len(result.NameServers) == 0 {
		return nil, fmt.Errorf("godaddy 未返回 NS")
	}
	return result.NameServers, nil
}

func (c *apiClient) goDaddySetNameServers(ctx context.Context, cfg config.GoDaddyConfig, domain string, nameServers []string) error {
	if len(nameServers) == 0 {
		return fmt.Errorf("NS 不能为空")
	}
	endpoint := fmt.Sprintf("https://api.godaddy.com/v1/domains/%s/nameservers", domain)
	items := make([]goDaddyNameServer, 0, len(nameServers))
	for _, ns := range nameServers {
		if strings.TrimSpace(ns) == "" {
			continue
		}
		items = append(items, goDaddyNameServer{Name: ns})
	}
	if len(items) == 0 {
		return fmt.Errorf("NS 不能为空")
	}
	body, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("godaddy 序列化失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("godaddy 请求创建失败: %w", err)
	}
	applyGoDaddyAuth(req, cfg)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("godaddy 请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("godaddy 读取响应失败: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrDomainNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("godaddy 设置失败: %s", strings.TrimSpace(string(data)))
	}
	return nil
}

func (c *apiClient) goDaddyListDomains(ctx context.Context, cfg config.GoDaddyConfig) ([]string, error) {
	endpoint := "https://api.godaddy.com/v1/domains?limit=1000&offset=0"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("godaddy 请求创建失败: %w", err)
	}
	applyGoDaddyAuth(req, cfg)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("godaddy 请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("godaddy 读取响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("godaddy 响应异常: %s", strings.TrimSpace(string(data)))
	}
	var items []goDaddyDomainListItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("godaddy 解析失败: %w", err)
	}
	domains := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Domain) == "" {
			continue
		}
		domains = append(domains, item.Domain)
	}
	if len(domains) == 0 {
		return nil, fmt.Errorf("godaddy 未返回域名列表")
	}
	return domains, nil
}

func applyGoDaddyAuth(req *http.Request, cfg config.GoDaddyConfig) {
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", cfg.APIKey, cfg.APISecret))
	req.Header.Set("Accept", "application/json")
}

func splitDomain(domain string) (string, string, error) {
	trimmed := strings.TrimSuffix(strings.TrimSpace(domain), ".")
	if trimmed == "" {
		return "", "", fmt.Errorf("域名不能为空")
	}
	idx := strings.LastIndex(trimmed, ".")
	if idx <= 0 || idx == len(trimmed)-1 {
		return "", "", fmt.Errorf("域名格式不正确: %s", domain)
	}
	return trimmed[:idx], trimmed[idx+1:], nil
}
