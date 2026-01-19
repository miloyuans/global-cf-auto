package app

import (
	"context"
	"log"
	"strings"
	"time"

	"DomainC/domain"
	"DomainC/tools"
)

type WhoisClient interface {
	Query(ctx context.Context, domain string) (string, error)
}

type ExpiryCheckerService struct {
	Whois        WhoisClient
	Repo         domain.Repository
	AlertWithin  time.Duration
	RateLimit    time.Duration
	QueryTimeout time.Duration
}

func cacheKey(ds domain.DomainSource) string {
	return strings.ToLower(strings.TrimSpace(ds.Domain)) + "|" + strings.ToLower(strings.TrimSpace(ds.Source))
}
func (c *ExpiryCheckerService) Check(ctx context.Context, domains []domain.DomainSource) ([]domain.DomainSource, []domain.FailureRecord, error) {
	if c.Whois == nil {
		return nil, nil, ErrMissingDependencies
	}
	if c.AlertWithin == 0 {
		c.AlertWithin = 24 * time.Hour
	}

	// 可选：Repo 支持 expiry cache
	type ExpiryCacheRepo interface {
		LoadExpiryCache() ([]domain.DomainSource, error)
		SaveExpiryCache(domains []domain.DomainSource) error
	}

	normalize := func(s string) string {
		s = strings.TrimSpace(strings.ToLower(s))
		s = strings.TrimSuffix(s, ".")
		return s
	}
	cacheKey := func(ds domain.DomainSource) string {
		return normalize(ds.Domain) + "|" + normalize(ds.Source)
	}

	// 1) 加载缓存：key=domain|source -> expiry(YYYY-MM-DD)
	var (
		cacheRepo  ExpiryCacheRepo
		cacheMap   map[string]string
		cacheDirty bool
	)
	if c.Repo != nil {
		if cr, ok := c.Repo.(ExpiryCacheRepo); ok {
			cacheRepo = cr
			cached, err := cr.LoadExpiryCache()
			if err != nil {
				log.Printf("[expiry] cache_load_failed err=%v", err)
			} else {
				cacheMap = make(map[string]string, len(cached))
				for _, ds := range cached {
					k := cacheKey(ds)
					if k == "|" {
						continue
					}
					exp := strings.TrimSpace(ds.Expiry)
					if exp == "" {
						continue
					}
					cacheMap[k] = exp
				}
				log.Printf("[expiry] cache_loaded size=%d", len(cacheMap))
			}
		}
	}

	// 2) 限速 ticker
	var ticker *time.Ticker
	if c.RateLimit > 0 {
		ticker = time.NewTicker(c.RateLimit)
		defer ticker.Stop()
	}

	var expiring []domain.DomainSource
	var failures []domain.FailureRecord

	// 3) 主循环
	for i, ds := range domains {
		ds.Domain = strings.TrimSpace(ds.Domain)
		ds.Source = strings.TrimSpace(ds.Source)
		ds.Expiry = strings.TrimSpace(ds.Expiry)

		if ds.Domain == "" {
			// 没有域名，直接算失败（你也可以选择 continue）
			failures = append(failures, domain.FailureRecord{Domain: ds.Domain, Source: ds.Source})
			continue
		}

		// A) ds 自带 Expiry：优先使用
		if ds.Expiry != "" {
			expiryTime, err := time.Parse("2006-01-02", ds.Expiry)
			if err != nil {
				failures = append(failures, domain.FailureRecord{
					Domain: ds.Domain,
					Source: ds.Source,
				})
				continue
			}

			// 写入 cache（可选，但建议：让 cache 尽快丰满）
			if cacheMap != nil {
				cacheMap[cacheKey(ds)] = expiryTime.Format("2006-01-02")
				cacheDirty = true
			}

			if time.Until(expiryTime) <= c.AlertWithin {
				ds.Expiry = expiryTime.Format("2006-01-02")
				expiring = append(expiring, ds)
			}
			continue
		}

		// B) 查 cache：没到阈值就跳过；到阈值/坏数据就删 cache 并进入重查
		if cacheMap != nil {
			if expStr, ok := cacheMap[cacheKey(ds)]; ok && strings.TrimSpace(expStr) != "" {
				if t, err := time.Parse("2006-01-02", strings.TrimSpace(expStr)); err == nil {
					// 还没到阈值：跳过
					if time.Until(t) > c.AlertWithin {
						log.Printf("[expiry] cache_hit_skip domain=%s source=%s expiry=%s", ds.Domain, ds.Source, expStr)
						continue
					}
					// 到阈值：删 cache，触发重查
					delete(cacheMap, cacheKey(ds))
					cacheDirty = true
					log.Printf("[expiry] cache_hit_recheck domain=%s source=%s expiry=%s", ds.Domain, ds.Source, expStr)
				} else {
					// cache 日期坏了：删掉，重查
					delete(cacheMap, cacheKey(ds))
					cacheDirty = true
					log.Printf("[expiry] cache_bad_recheck domain=%s source=%s expiry=%s", ds.Domain, ds.Source, expStr)
				}
			}
		}

		// C) 限速
		if i > 0 && ticker != nil {
			select {
			case <-ctx.Done():
				return expiring, failures, ctx.Err()
			case <-ticker.C:
			}
		}

		// D) 查询 WHOIS / RDAP
		lookupCtx := ctx
		cancel := func() {}
		if c.QueryTimeout > 0 {
			lookupCtx, cancel = context.WithTimeout(ctx, c.QueryTimeout)
		}

		result, err := c.Whois.Query(lookupCtx, ds.Domain)
		cancel()

		if err != nil {
			log.Printf("[expiry] lookup_failed domain=%s source=%s", ds.Domain, ds.Source)
			failures = append(failures, domain.FailureRecord{
				Domain: ds.Domain,
				Source: ds.Source,
			})
			continue
		}

		// E) 解析到期日期（优先直接 yyyy-mm-dd）
		result = strings.TrimSpace(result)

		var expiryTime time.Time
		if t, err := time.Parse("2006-01-02", result); err == nil {
			expiryTime = t
		} else {
			// 再从原始文本抽取
			expiry, ok := tools.ExtractExpiry(result)
			if !ok {
				log.Printf("[expiry] extract_failed domain=%s source=%s", ds.Domain, ds.Source)
				failures = append(failures, domain.FailureRecord{
					Domain: ds.Domain,
					Source: ds.Source,
				})
				continue
			}

			t, err := time.Parse("2006-01-02", strings.TrimSpace(expiry))
			if err != nil {
				log.Printf("[expiry] parse_failed domain=%s source=%s expiry=%q", ds.Domain, ds.Source, expiry)
				failures = append(failures, domain.FailureRecord{
					Domain: ds.Domain,
					Source: ds.Source,
				})
				continue
			}
			expiryTime = t
		}

		// F) 写入 cache（查到日期就缓存）
		expStr := expiryTime.Format("2006-01-02")
		if cacheMap != nil {
			cacheMap[cacheKey(ds)] = expStr
			cacheDirty = true
		}

		// G) 达到阈值：加入 expiring
		if time.Until(expiryTime) <= c.AlertWithin {
			ds.Expiry = expStr
			expiring = append(expiring, ds)
		}
	}

	// 4) 保存 expiring / failures（保持你原逻辑）
	if c.Repo != nil {
		if err := c.Repo.SaveExpiring(expiring); err != nil {
			return expiring, failures, err
		}
		if err := c.Repo.SaveFailures(failures); err != nil {
			return expiring, failures, err
		}
	}

	// 5) 保存 cache（可选接口存在且有变化才写）
	if cacheRepo != nil && cacheMap != nil && cacheDirty {
		out := make([]domain.DomainSource, 0, len(cacheMap))
		for k, exp := range cacheMap {
			parts := strings.SplitN(k, "|", 2)
			if len(parts) != 2 {
				continue
			}
			if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(exp) == "" {
				continue
			}
			out = append(out, domain.DomainSource{
				Domain: parts[0],
				Source: parts[1],
				Expiry: exp,
			})
		}
		if err := cacheRepo.SaveExpiryCache(out); err != nil {
			log.Printf("[expiry] cache_save_failed err=%v", err)
		} else {
			log.Printf("[expiry] cache_saved size=%d", len(out))
		}
	}

	return expiring, failures, nil
}
