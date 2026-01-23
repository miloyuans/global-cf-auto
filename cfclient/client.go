package cfclient

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"DomainC/config"

	cloudflare "github.com/cloudflare/cloudflare-go"
)

// DomainInfo 是 cfclient 层的域名描述，避免直接依赖 domain 包
type DomainInfo struct {
	Domain string
	Source string
	IsCF   bool
	Status string
	Paused bool
}
type ZoneDetail struct {
	ID          string
	Name        string
	NameServers []string
	Status      string
	Paused      bool
}
type OriginCert struct {
	ID             string
	CertificatePEM string
	PrivateKeyPEM  string
	CSRPEM         string
	Hostnames      []string
	RequestType    string
	RequestedDays  int
	ExpiresOn      time.Time
}
type OriginCACertInfo struct {
	ID          string
	Hostnames   []string
	ExpiresOn   time.Time
	RevokedAt   *time.Time
	RequestType string
}

// Client 定义了 Cloudflare 相关操作的抽象接口
type Client interface {
	FetchAllDomains(ctx context.Context, account config.CF) ([]DomainInfo, error)
	ListDNSRecords(ctx context.Context, account config.CF, domain string) ([]cloudflare.DNSRecord, error)
	PauseDomain(ctx context.Context, account config.CF, domain string, pause bool) error
	DeleteDomain(ctx context.Context, account config.CF, domain string) error
	GetZoneDetails(ctx context.Context, account config.CF, domain string) (ZoneDetail, error)
	CreateZone(ctx context.Context, account config.CF, domain string) (ZoneDetail, error)
	UpsertDNSRecord(ctx context.Context, account config.CF, domain string, params DNSRecordParams) (cloudflare.DNSRecord, error)
	DeleteDNSRecord(ctx context.Context, account config.CF, domain string, recordName string) (int, error)
	ListZones(ctx context.Context, acc config.CF) ([]ZoneDetail, error)
	CreateOriginCertificate(ctx context.Context, account config.CF, hostnames []string) (OriginCert, error)
	ListOriginCACertificates(ctx context.Context, account config.CF) ([]OriginCACertInfo, error)
	PurgeZoneCache(ctx context.Context, account config.CF, zoneID string) error
	ListCustomLists(ctx context.Context, account config.CF) ([]cloudflare.List, error)
	GetCustomList(ctx context.Context, account config.CF, listID string) (cloudflare.List, error)
	ListCustomListItems(ctx context.Context, account config.CF, listID string) ([]cloudflare.ListItem, error)
	CreateCustomListItem(ctx context.Context, account config.CF, listID string, ip string, comment string) ([]cloudflare.ListItem, error)
	DeleteCustomListItem(ctx context.Context, account config.CF, listID string, itemID string) ([]cloudflare.ListItem, error)
}

type apiClient struct{}

// NewClient 返回默认的 Cloudflare API 客户端实现
func NewClient() Client {
	return &apiClient{}
}

// ErrZoneNotFound 在账户中未找到域名时返回
var ErrZoneNotFound = errors.New("zone not found")

// DNSRecordParams 描述需要创建或更新的解析记录
type DNSRecordParams struct {
	Type    string
	Name    string
	Content string
	Proxied bool
	TTL     int
}

// GetAccountID 获取指定账户的 Account ID（取第一个）
func (c *apiClient) GetAccountID(ctx context.Context, account config.CF) (string, error) {
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return "", fmt.Errorf("初始化客户端失败 [%s]: %v", account.Label, err)
	}

	accounts, _, err := api.Accounts(ctx, cloudflare.AccountsListParams{})
	if err != nil {
		return "", fmt.Errorf("获取账户列表失败 [%s]: %v", account.Label, err)
	}
	if len(accounts) == 0 {
		return "", fmt.Errorf("账户 [%s] 下无可用 Account ID", account.Label)
	}
	return accounts[0].ID, nil
}

// CreateZone 创建新 Zone（修复版：自动获取并传递 Account ID + 重试）
func (c *apiClient) CreateZone(ctx context.Context, account config.CF, domain string) (ZoneDetail, error) {
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return ZoneDetail{}, fmt.Errorf("初始化 Cloudflare 客户端失败 [%s]: %v", account.Label, err)
	}

	// 获取 Account ID
	accountID, err := c.GetAccountID(ctx, account)
	if err != nil {
		return ZoneDetail{}, err
	}

	var zone cloudflare.Zone
	var createErr error
	maxRetries := 5
	for attempt := 0; attempt < maxRetries; attempt++ {
		zone, createErr = api.CreateZone(ctx, domain, false, cloudflare.Account{ID: accountID}, "full")
		if createErr == nil {
			break
		}
		if strings.Contains(createErr.Error(), "500") || strings.Contains(createErr.Error(), "Internal Server Error") {
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
			continue
		}
		break
	}

	if createErr != nil {
		if strings.Contains(createErr.Error(), "500") {
			return ZoneDetail{}, fmt.Errorf("创建域名失败（HTTP 500，很可能是域名尚未在注册商成功注册）: %w\n请先在 Namecheap/GoDaddy/阿里云 等注册域名，等待 WHOIS 可查后再试", createErr)
		}
		return ZoneDetail{}, fmt.Errorf("创建域名失败: %w", createErr)
	}

	return ZoneDetail{
		ID:          zone.ID,
		Name:        zone.Name,
		NameServers: zone.NameServers,
		Status:      zone.Status,
		Paused:      zone.Paused,
	}, nil
}
func (c *apiClient) DeleteDNSRecord(ctx context.Context, account config.CF, domain string, recordName string) (int, error) {
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return 0, fmt.Errorf("初始化客户端失败 [%s]: %v", account.Label, err)
	}

	zones, err := api.ListZonesContext(ctx, cloudflare.WithZoneFilters(domain, "", ""))
	if err != nil {
		return 0, fmt.Errorf("获取 Zone 失败: %v", err)
	}
	if len(zones.Result) == 0 {
		return 0, fmt.Errorf("%w: %s", ErrZoneNotFound, domain)
	}
	zoneID := cloudflare.ZoneIdentifier(zones.Result[0].ID)

	searchParams := cloudflare.ListDNSRecordsParams{
		Name: fqdn(recordName, domain),
	}
	records, _, err := api.ListDNSRecords(ctx, zoneID, searchParams)
	if err != nil {
		return 0, fmt.Errorf("查询解析记录失败: %v", err)
	}
	if len(records) == 0 {
		return 0, nil
	}

	deleted := 0
	for _, record := range records {
		if err := api.DeleteDNSRecord(ctx, zoneID, record.ID); err != nil {
			return deleted, fmt.Errorf("删除解析记录失败: %v", err)
		}
		deleted++
	}

	return deleted, nil
}
func (c *apiClient) PurgeZoneCache(ctx context.Context, account config.CF, zoneID string) error {
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return fmt.Errorf("初始化 Cloudflare 客户端失败 [%s]: %v", account.Label, err)
	}
	_, err = api.PurgeCache(ctx, zoneID, cloudflare.PurgeCacheRequest{Everything: true})
	if err != nil {
		return fmt.Errorf("清理缓存失败: %v", err)
	}
	return nil
}

// GetZoneDetails 获取 Zone 详情（修复版：使用 Account ID 过滤）
func (c *apiClient) GetZoneDetails(ctx context.Context, account config.CF, domain string) (ZoneDetail, error) {
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return ZoneDetail{}, fmt.Errorf("初始化客户端失败 [%s]: %v", account.Label, err)
	}

	accountID, err := c.GetAccountID(ctx, account)
	if err != nil {
		return ZoneDetail{}, err
	}

	zones, err := api.ListZonesContext(ctx, cloudflare.WithZoneFilters(domain, accountID, ""))
	if err != nil {
		return ZoneDetail{}, fmt.Errorf("获取 Zone 失败 [%s]: %v", account.Label, err)
	}
	if len(zones.Result) == 0 {
		return ZoneDetail{}, ErrZoneNotFound
	}

	z := zones.Result[0]
	return ZoneDetail{
		ID:          z.ID,
		Name:        z.Name,
		NameServers: z.NameServers,
		Status:      z.Status,
		Paused:      z.Paused,
	}, nil
}

// DeleteDomain 从 Cloudflare 删除 zone
func (c *apiClient) DeleteDomain(ctx context.Context, account config.CF, domain string) error {
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return fmt.Errorf("初始化 Cloudflare 客户端失败 [%s]: %v", account.Label, err)
	}

	zones, err := api.ListZonesContext(ctx, cloudflare.WithZoneFilters(domain, "", ""))
	if err != nil {
		return fmt.Errorf("获取 Zone 失败: %v", err)
	}
	if len(zones.Result) == 0 {
		return fmt.Errorf("%w: %s", ErrZoneNotFound, domain)
	}

	zoneID := zones.Result[0].ID
	_, err = api.DeleteZone(ctx, zoneID)
	if err != nil {
		return fmt.Errorf("删除域名失败: %v", err)
	}
	return nil
}

// PauseDomain 暂停或恢复域名（兼容旧版 cloudflare-go SDK）
func (c *apiClient) PauseDomain(ctx context.Context, account config.CF, domain string, pause bool) error {
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return fmt.Errorf("初始化客户端失败 [%s]: %v", account.Label, err)
	}

	// 获取 Account ID 用于精确过滤
	accountID, err := c.GetAccountID(ctx, account)
	if err != nil {
		// 如果获取失败，降级使用空过滤（兼容性）
		accountID = ""
	}

	// 查找 Zone
	zones, err := api.ListZonesContext(ctx, cloudflare.WithZoneFilters(domain, accountID, ""))
	if err != nil {
		return fmt.Errorf("获取 Zone 失败: %v", err)
	}
	if len(zones.Result) == 0 {
		return fmt.Errorf("%w: %s", ErrZoneNotFound, domain)
	}

	zoneID := zones.Result[0].ID
	_, err = api.ZoneSetPaused(ctx, zoneID, pause)
	if err != nil {
		return fmt.Errorf("设置暂停状态失败（pause=%v）: %v", pause, err)
	}

	return nil
}

func (c *apiClient) ListDNSRecords(ctx context.Context, account config.CF, domain string) ([]cloudflare.DNSRecord, error) {
	log.Printf("[ListDNSRecords] start domain=%q account=%q", domain, account.Label)
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return nil, fmt.Errorf("初始化客户端失败 [%s]: %v", account.Label, err)
	}

	//1) 规范化 domain
	domain = strings.TrimSpace(strings.ToLower(domain))
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		return nil, fmt.Errorf("%w: empty domain", ErrZoneNotFound)
	}

	// 2) 生成候选 zone（支持子域输入）
	parts := strings.Split(domain, ".")
	cands := []string{domain}
	log.Printf("[ListDNSRecords] candidates=%v", cands)
	if len(parts) >= 2 {
		cands = cands[:0]
		for i := 0; i <= len(parts)-2; i++ {
			cands = append(cands, strings.Join(parts[i:], "."))
		}
	}

	// 3) 按候选逐个找 zone（优先精确匹配 Name）
	var zoneID string
	var matchedName string

	for _, cand := range cands {
		log.Printf("[ListDNSRecords] try cand=%q", cand)

		zones, err := api.ListZonesContext(ctx, cloudflare.WithZoneFilters(cand, "", ""))
		if err != nil {
			return nil, fmt.Errorf("获取 Zone 失败: %v", err)
		}
		if len(zones.Result) == 0 {
			continue
		}

		// 精确匹配 zone 名称（避免不完全等价的返回）
		for _, z := range zones.Result {
			if strings.EqualFold(z.Name, cand) {
				zoneID = z.ID
				matchedName = z.Name
				break
			}
		}
		// 没找到精确匹配就退而求其次取第一个
		if zoneID == "" {
			zoneID = zones.Result[0].ID
			matchedName = zones.Result[0].Name
		}
		break
	}
	log.Printf("[ListDNSRecords] zone matched name=%q id=%q", matchedName, zoneID)

	if zoneID == "" {
		return nil, fmt.Errorf("%w: %s", ErrZoneNotFound, domain)
	}

	records, _, err := api.ListDNSRecords(ctx, cloudflare.ZoneIdentifier(zoneID), cloudflare.ListDNSRecordsParams{})
	if err != nil {
		return nil, fmt.Errorf("列出 DNS 记录失败(Zone=%s, ID=%s): %v", matchedName, zoneID, err)
	}
	log.Printf("[ListDNSRecords] records=%d", len(records))
	return records, nil
}

// fqdn 规范化 Cloudflare record name
func fqdn(name, zone string) string {
	name = strings.TrimSpace(name)
	zone = strings.TrimSuffix(strings.ToLower(zone), ".")

	if name == "@" || name == "" {
		return zone
	}

	// 已经是完整域名
	nameLower := strings.ToLower(strings.TrimSuffix(name, "."))
	if nameLower == zone || strings.HasSuffix(nameLower, "."+zone) {
		return nameLower
	}

	return strings.ToLower(name) + "." + zone
}

func (c *apiClient) UpsertDNSRecord(
	ctx context.Context,
	account config.CF,
	domain string,
	params DNSRecordParams,
) (cloudflare.DNSRecord, error) {
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return cloudflare.DNSRecord{}, fmt.Errorf("初始化客户端失败 [%s]: %v", account.Label, err)
	}

	zones, err := api.ListZonesContext(ctx, cloudflare.WithZoneFilters(domain, "", ""))
	if err != nil {
		return cloudflare.DNSRecord{}, fmt.Errorf("获取 Zone 失败: %v", err)
	}
	if len(zones.Result) == 0 {
		return cloudflare.DNSRecord{}, fmt.Errorf("%w: %s", ErrZoneNotFound, domain)
	}
	zone := zones.Result[0]
	zoneID := cloudflare.ZoneIdentifier(zone.ID)

	// 关键：把传入的 @ / www 变成 CF 里可稳定匹配的 FQDN
	recordName := fqdn(params.Name, domain)

	searchParams := cloudflare.ListDNSRecordsParams{
		Type: strings.ToUpper(params.Type),
		Name: recordName,
	}

	existing, _, err := api.ListDNSRecords(ctx, zoneID, searchParams)
	if err != nil {
		return cloudflare.DNSRecord{}, fmt.Errorf("查询解析记录失败: %v", err)
	}

	ttl := params.TTL
	if ttl <= 0 {
		ttl = 1 // auto
	}
	proxied := params.Proxied

	// 存在：更新第一条，并删除其它重复的（真正“替换掉”）
	if len(existing) > 0 {
		target := existing[0]

		record, err := api.UpdateDNSRecord(ctx, zoneID, cloudflare.UpdateDNSRecordParams{
			ID:      target.ID,
			Type:    searchParams.Type,
			Name:    recordName,
			Content: params.Content,
			TTL:     ttl,
			Proxied: &proxied,
		})
		if err != nil {
			return cloudflare.DNSRecord{}, fmt.Errorf("更新解析记录失败: %v", err)
		}

		// 可选但强烈建议：清理同名重复记录
		for i := 1; i < len(existing); i++ {
			_ = api.DeleteDNSRecord(ctx, zoneID, existing[i].ID)
		}

		return record, nil
	}

	// 不存在：创建
	record, err := api.CreateDNSRecord(ctx, zoneID, cloudflare.CreateDNSRecordParams{
		Type:    searchParams.Type,
		Name:    recordName,
		Content: params.Content,
		TTL:     ttl,
		Proxied: &proxied,
	})
	if err != nil {
		return cloudflare.DNSRecord{}, fmt.Errorf("创建解析记录失败: %v", err)
	}

	return record, nil
}

func (c *apiClient) FetchAllDomains(ctx context.Context, account config.CF) ([]DomainInfo, error) {

	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return nil, fmt.Errorf(
			"初始化 Cloudflare 客户端失败 [%s]: %v",
			account.Label, err,
		)
	}

	zones, err := api.ListZonesContext(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"获取域名失败 [%s]: %v",
			account.Label, err,
		)
	}

	out := make([]DomainInfo, 0, len(zones.Result))

	for _, z := range zones.Result {
		out = append(out, DomainInfo{
			Domain: z.Name,
			Source: account.Label,
			IsCF:   true,
			Status: z.Status,
			Paused: z.Paused,
		})
	}

	return out, nil
}

func FetchAllDomains(account config.CF) ([]DomainInfo, error) {
	return NewClient().FetchAllDomains(context.Background(), account)
}

// GetAccountByLabel 返回配置中与 label 匹配的 Cloudflare 账号指针，找不到则返回 nil
func GetAccountByLabel(label string) *config.CF {
	for i := range config.Cfg.CloudflareAccounts {
		if config.Cfg.CloudflareAccounts[i].Label == label {
			return &config.Cfg.CloudflareAccounts[i]
		}
	}
	return nil
}

func ensureTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, 30*time.Second)
}
func (c *apiClient) ListZones(ctx context.Context, account config.CF) ([]ZoneDetail, error) {
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return nil, fmt.Errorf("初始化 Cloudflare 客户端失败 [%s]: %v", account.Label, err)
	}

	accountID, err := c.GetAccountID(ctx, account)
	if err != nil {
		return nil, err
	}

	zones, err := api.ListZonesContext(ctx, cloudflare.WithZoneFilters("", accountID, ""))
	if err != nil {
		return nil, fmt.Errorf("列出 Zone 失败 [%s]: %v", account.Label, err)
	}

	out := make([]ZoneDetail, 0, len(zones.Result))
	for _, z := range zones.Result {
		out = append(out, ZoneDetail{
			ID:          z.ID,
			Name:        z.Name,
			NameServers: z.NameServers,
			Status:      z.Status,
			Paused:      z.Paused,
		})
	}
	return out, nil
}

func (c *apiClient) CreateOriginCertificate(ctx context.Context, account config.CF, hostnames []string) (OriginCert, error) {

	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	hostnames = normalizeHostnames(hostnames)
	if len(hostnames) == 0 {
		return OriginCert{}, fmt.Errorf("hostnames 不能为空")
	}

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return OriginCert{}, fmt.Errorf(
			"初始化 Cloudflare 客户端失败 [%s]: %v",
			account.Label, err,
		)
	}

	// 1. 生成 RSA 私钥
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return OriginCert{}, fmt.Errorf("生成私钥失败: %v", err)
	}

	// 2. 生成 CSR（SAN = hostnames）
	csrPEM, err := buildCSRPEM(priv, hostnames)
	if err != nil {
		return OriginCert{}, err
	}

	// 3. 调用 CreateOriginCACertificate（⚠️ 用你这个 struct）
	req := cloudflare.CreateOriginCertificateParams{
		CSR:             csrPEM,
		Hostnames:       hostnames,
		RequestType:     "origin-rsa",
		RequestValidity: 5475, // 15 年
	}

	cert, err := api.CreateOriginCACertificate(ctx, req)
	if err != nil {
		return OriginCert{}, fmt.Errorf("创建 Origin CA 证书失败: %v", err)
	}

	// 4. 私钥 PEM
	keyPEM, err := encodeRSAPrivateKeyPEM(priv)
	if err != nil {
		return OriginCert{}, err
	}

	return OriginCert{
		ID:             cert.ID,
		CertificatePEM: cert.Certificate,
		PrivateKeyPEM:  keyPEM,
		CSRPEM:         csrPEM,
		Hostnames:      cert.Hostnames,
		RequestType:    cert.RequestType,
		RequestedDays:  cert.RequestValidity,
		ExpiresOn:      cert.ExpiresOn,
	}, nil
}
func normalizeHostnames(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, h := range in {
		h = strings.TrimSpace(strings.ToLower(h))
		if h == "" {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	return out
}

func buildCSRPEM(priv *rsa.PrivateKey, hostnames []string) (string, error) {
	// CN 用第一个 hostname（只是展示用途，实际以 SAN 为准）
	cn := hostnames[0]

	tpl := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: cn,
		},
		DNSNames: hostnames,
	}

	der, err := x509.CreateCertificateRequest(rand.Reader, tpl, priv)
	if err != nil {
		return "", fmt.Errorf("生成 CSR 失败: %v", err)
	}

	block := &pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}
	return string(pem.EncodeToMemory(block)), nil
}

func encodeRSAPrivateKeyPEM(priv *rsa.PrivateKey) (string, error) {
	der := x509.MarshalPKCS1PrivateKey(priv)
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}
	return string(pem.EncodeToMemory(block)), nil
}
func (c *apiClient) ListOriginCACertificates(ctx context.Context, account config.CF) ([]OriginCACertInfo, error) {
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return nil, fmt.Errorf("初始化 Cloudflare 客户端失败 [%s]: %v", account.Label, err)
	}

	// params 结构你本地 SDK 里一定有，不同版本字段略不同：
	// 一般至少可空 struct 或带分页；不行就传 cloudflare.ListOriginCertificatesParams{}
	certs, err := api.ListOriginCACertificates(ctx, cloudflare.ListOriginCertificatesParams{})
	if err != nil {
		return nil, fmt.Errorf("列出 Origin CA 证书失败 [%s]: %v", account.Label, err)
	}

	out := make([]OriginCACertInfo, 0, len(certs))
	for _, c := range certs {
		var revoked *time.Time
		if !c.RevokedAt.IsZero() {
			t := c.RevokedAt
			revoked = &t
		}
		out = append(out, OriginCACertInfo{
			ID:          c.ID,
			Hostnames:   c.Hostnames,
			ExpiresOn:   c.ExpiresOn,
			RevokedAt:   revoked,
			RequestType: c.RequestType,
		})
	}
	return out, nil
}
func (c *apiClient) ListCustomLists(ctx context.Context, account config.CF) ([]cloudflare.List, error) {
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return nil, fmt.Errorf("初始化 Cloudflare 客户端失败 [%s]: %v", account.Label, err)
	}

	accountID, err := c.GetAccountID(ctx, account)
	if err != nil {
		return nil, err
	}

	lists, err := api.ListLists(ctx, cloudflare.AccountIdentifier(accountID), cloudflare.ListListsParams{})
	if err != nil {
		return nil, fmt.Errorf("列出账号 %s Custom Lists 失败: %v", account.Label, err)
	}

	var out []cloudflare.List
	for _, list := range lists {
		if list.Kind == cloudflare.ListTypeIP {
			out = append(out, list)
		}
	}
	return out, nil
}

func (c *apiClient) GetCustomList(ctx context.Context, account config.CF, listID string) (cloudflare.List, error) {
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return cloudflare.List{}, fmt.Errorf("初始化 Cloudflare 客户端失败 [%s]: %v", account.Label, err)
	}
	accountID, err := c.GetAccountID(ctx, account)
	if err != nil {
		return cloudflare.List{}, err
	}

	list, err := api.GetList(ctx, cloudflare.AccountIdentifier(accountID), listID)
	if err != nil {
		return cloudflare.List{}, fmt.Errorf("获取 Custom List 失败 [%s]: %v", account.Label, err)
	}
	return list, nil
}

func (c *apiClient) ListCustomListItems(ctx context.Context, account config.CF, listID string) ([]cloudflare.ListItem, error) {
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	if strings.TrimSpace(listID) == "" {
		return nil, errors.New("listID is empty")
	}

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return nil, fmt.Errorf("初始化 Cloudflare 客户端失败 [%s]: %v", account.Label, err)
	}

	accountID, err := c.GetAccountID(ctx, account)
	if err != nil {
		return nil, err
	}

	items, err := api.ListListItems(ctx, cloudflare.AccountIdentifier(accountID), cloudflare.ListListItemsParams{
		ID:      listID,
		PerPage: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("列出 Custom List 条目失败 [%s]: %v", account.Label, err)
	}
	return items, nil
}

func (c *apiClient) CreateCustomListItem(ctx context.Context, account config.CF, listID string, ip string, comment string) ([]cloudflare.ListItem, error) {
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	if strings.TrimSpace(listID) == "" {
		return nil, errors.New("listID is empty")
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil, errors.New("ip is empty")
	}

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return nil, fmt.Errorf("初始化 Cloudflare 客户端失败 [%s]: %v", account.Label, err)
	}

	accountID, err := c.GetAccountID(ctx, account)
	if err != nil {
		return nil, err
	}

	items, err := api.CreateListItem(ctx, cloudflare.AccountIdentifier(accountID), cloudflare.ListCreateItemParams{
		ID: listID,
		Item: cloudflare.ListItemCreateRequest{
			IP:      &ip,
			Comment: comment,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("添加 Custom List 条目失败 [%s]: %v", account.Label, err)
	}
	return items, nil
}

func (c *apiClient) DeleteCustomListItem(ctx context.Context, account config.CF, listID string, itemID string) ([]cloudflare.ListItem, error) {
	ctx, cancel := ensureTimeout(ctx)
	defer cancel()

	if strings.TrimSpace(listID) == "" {
		return nil, errors.New("listID is empty")
	}
	if strings.TrimSpace(itemID) == "" {
		return nil, errors.New("itemID is empty")
	}

	api, err := cloudflare.NewWithAPIToken(account.APIToken)
	if err != nil {
		return nil, fmt.Errorf("初始化 Cloudflare 客户端失败 [%s]: %v", account.Label, err)
	}

	accountID, err := c.GetAccountID(ctx, account)
	if err != nil {
		return nil, err
	}

	items, err := api.DeleteListItems(ctx, cloudflare.AccountIdentifier(accountID), cloudflare.ListDeleteItemsParams{
		ID: listID,
		Items: cloudflare.ListItemDeleteRequest{Items: []cloudflare.ListItemDeleteItemRequest{
			{ID: itemID},
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("删除 Custom List 条目失败 [%s]: %v", account.Label, err)
	}
	return items, nil
}
