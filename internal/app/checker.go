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

func (c *ExpiryCheckerService) Check(
	ctx context.Context,
	domains []domain.DomainSource,
) ([]domain.DomainSource, []domain.FailureRecord, error) {

	if c.Whois == nil {
		return nil, nil, ErrMissingDependencies
	}
	if c.AlertWithin == 0 {
		c.AlertWithin = 24 * time.Hour
	}

	var ticker *time.Ticker
	if c.RateLimit > 0 {
		ticker = time.NewTicker(c.RateLimit)
		defer ticker.Stop()
	}

	var expiring []domain.DomainSource
	var failures []domain.FailureRecord

	for i, ds := range domains {

		// 已有到期时间，直接解析
		if expiryStr := strings.TrimSpace(ds.Expiry); expiryStr != "" {
			expiryTime, err := time.Parse("2006-01-02", expiryStr)
			if err != nil {
				failures = append(failures, domain.FailureRecord{
					Domain: ds.Domain,
					Source: ds.Source,
				})
				continue
			}

			if time.Until(expiryTime) <= c.AlertWithin {
				ds.Expiry = expiryTime.Format("2006-01-02")
				expiring = append(expiring, ds)
			}
			continue
		}

		// 限速
		if i > 0 && ticker != nil {
			select {
			case <-ctx.Done():
				return expiring, failures, ctx.Err()
			case <-ticker.C:
			}
		}

		// 查询 WHOIS / RDAP（由 client 决定返回什么）
		lookupCtx := ctx
		cancel := func() {}
		if c.QueryTimeout > 0 {
			lookupCtx, cancel = context.WithTimeout(ctx, c.QueryTimeout)
		}
		result, err := c.Whois.Query(lookupCtx, ds.Domain)
		cancel()

		if err != nil {
			// 只记录失败域名；原因不落库（日志可留最小信息）
			log.Printf("[expiry] lookup_failed domain=%s", ds.Domain)
			failures = append(failures, domain.FailureRecord{
				Domain: ds.Domain,
				Source: ds.Source,
			})
			continue
		}

		if t, err := time.Parse("2006-01-02", strings.TrimSpace(result)); err == nil {
			if time.Until(t) <= c.AlertWithin {
				ds.Expiry = t.Format("2006-01-02")
				expiring = append(expiring, ds)
			}
			continue
		}

		expiry, ok := tools.ExtractExpiry(result)
		if !ok {
			log.Printf("[expiry] extract_failed domain=%s", ds.Domain)
			failures = append(failures, domain.FailureRecord{
				Domain: ds.Domain,
				Source: ds.Source,
			})
			continue
		}

		expiryTime, err := time.Parse("2006-01-02", expiry)
		if err != nil {
			log.Printf("[expiry] parse_failed domain=%s", ds.Domain)
			failures = append(failures, domain.FailureRecord{
				Domain: ds.Domain,
				Source: ds.Source,
			})
			continue
		}

		if time.Until(expiryTime) <= c.AlertWithin {
			ds.Expiry = expiry
			expiring = append(expiring, ds)
		}
	}

	if c.Repo != nil {
		if err := c.Repo.SaveExpiring(expiring); err != nil {
			return expiring, failures, err
		}
		if err := c.Repo.SaveFailures(failures); err != nil {
			return expiring, failures, err
		}
	}

	return expiring, failures, nil
}
