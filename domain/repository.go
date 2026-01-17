package domain

// Repository 统一管理域名列表及即将到期缓存的持久化。
type Repository interface {
	// LoadSources 读取域名来源列表，例如本地文件或数据库。
	LoadSources() ([]DomainSource, error)
	// SaveExpiring 写入即将到期的域名列表，便于后续通知或缓存。
	SaveExpiring(domains []DomainSource) error
	// SaveFailures 写入解析失败的域名列表，便于排查与人工处理。
	SaveFailures(failures []FailureRecord) error
	// 从缓存文件读
	LoadExpiryCache() ([]DomainSource, error)
	// 覆盖写回缓存文件
	SaveExpiryCache(domains []DomainSource) error
}
