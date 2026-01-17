package tools

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/likexian/whois"
	"github.com/openrdap/rdap"
)

var expiryRegex = regexp.MustCompile(
	`(?i)\b(expiration date|expiration|expiry|expires|expires on|registry expiry date|registry expiration date|paid-till)\b[^0-9A-Za-z]*([0-9A-Za-z ,:/\-T\.Z+]+)`,
)

var layouts = []string{
	"2006-01-02",
	"2006/01/02",
	"2006.01.02",
	"02-Jan-2006",
	"Jan 02, 2006",
	"January 2 2006",
	"January 02 2006",
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02T15:04:05Z",
	"2006-01-02 15:04:05",
}

func parseWithLayouts(dateStr string) (string, bool) {
	cleaned := strings.TrimSpace(strings.Trim(dateStr, ":"))
	cleaned = strings.Join(strings.Fields(cleaned), " ")

	for _, layout := range layouts {
		if t, err := time.Parse(layout, cleaned); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}

// 保持原函数签名：不打 domain 日志，只做纯提取
func ExtractExpiry(result string) (string, bool) {
	if match := expiryRegex.FindStringSubmatch(result); len(match) >= 3 {
		if parsed, ok := parseWithLayouts(match[2]); ok {
			return parsed, true
		}
	}
	return "", false
}

// 新增：带 domain 的提取（关键位置打日志，失败只返回 ok=false）
func ExtractExpiryWithDomain(domain string, result string) (string, bool) {
	expiry, ok := ExtractExpiry(result)
	if ok {
		log.Printf("[expiry] extracted domain=%s expiry=%s", domain, expiry)
		return expiry, true
	}

	// 不返回失败原因，只在日志里记录一点点上下文（避免过长）
	snippet := strings.ReplaceAll(result, "\r\n", "\n")
	if len(snippet) > 300 {
		snippet = snippet[:300] + " ... (truncated)"
	}
	log.Printf("[expiry] NOT FOUND domain=%s whois_snippet=%q", domain, snippet)
	return "", false
}

// 建议：CheckWhois 改为“只负责拿到期日”，失败 ok=false；原因只打日志
func CheckWhois(domain string) (string, bool) {
	client := &rdap.Client{}

	// 1) RDAP 优先
	d, err := client.QueryDomain(domain)
	if err == nil && d != nil {
		for _, event := range d.Events {
			if strings.EqualFold(event.Action, "expiration") {
				// RDAP event.Date 往往是 RFC3339，统一转成 2006-01-02
				if parsed, ok := parseWithLayouts(event.Date); ok {
					log.Printf("[rdap] success domain=%s expiry=%s raw=%s", domain, parsed, event.Date)
					return parsed, true
				}
				log.Printf("[rdap] parse_failed domain=%s raw=%s", domain, event.Date)
				break
			}
		}
	} else if err != nil {
		log.Printf("[rdap] query_failed domain=%s err=%v", domain, err)
	}

	// 2) WHOIS
	result, err := whois.Whois(domain)
	if err != nil {
		log.Printf("[whois] query_failed domain=%s err=%v", domain, err)
		return "", false
	}

	result = strings.ReplaceAll(result, "\r\n", "\n")

	if expiry, ok := ExtractExpiryWithDomain(domain, result); ok {
		log.Printf("[whois] success domain=%s expiry=%s", domain, expiry)
		return expiry, true
	}

	log.Printf("[whois] expiry_not_found domain=%s", domain)
	return "", false
}

func DaysUntilExpiry(expiry string) (int, error) {
	expiryTime, err := time.Parse("2006-01-02", expiry)
	if err != nil {
		log.Printf("[expiry] days_parse_failed expiry=%q err=%v", expiry, err)
		return -1, fmt.Errorf("parse expiry failed")
	}
	days := int(time.Until(expiryTime).Hours() / 24)
	return days, nil
}
