package tools

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/likexian/whois"
	"github.com/openrdap/rdap"
)

func ExtractExpiry(result string) (string, bool) {
	var expiryRegex = regexp.MustCompile(
		`(?i)\b(expiration date|expiration|expiry|expires|expires on|registry expiry date|registry expiration date|paid-till)\b[^0-9A-Za-z]*([0-9A-Za-z ,:/\-T\.Z+]+)`,
	)
	// 兜底匹配常见的日期格式，避免服务响应中没有明确的关键字。
	// var dateRegex = regexp.MustCompile(
	// 	`\b\d{4}-\d{2}-\d{2}(?:[ T]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)?\b`,
	// )
	// 先不要启用兜底匹配，避免误伤其他日期字段

	layouts := []string{
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

	parseWithLayouts := func(dateStr string) (string, bool) {
		cleaned := strings.TrimSpace(strings.Trim(dateStr, ":"))
		cleaned = strings.Join(strings.Fields(cleaned), " ")

		for _, layout := range layouts {
			if t, err := time.Parse(layout, cleaned); err == nil {
				return t.Format("2006-01-02"), true
			}
		}

		return "", false
	}

	if match := expiryRegex.FindStringSubmatch(result); len(match) >= 3 {
		if parsed, ok := parseWithLayouts(match[2]); ok {
			return parsed, true
		}
	}

	// if match := dateRegex.FindString(result); match != "" {
	// 	if parsed, ok := parseWithLayouts(match); ok {
	// 		return parsed, true
	// 	}
	// }

	return "", false
}

type Button struct {
	Text         string
	CallbackData string
}

func CheckWhois(domain string) string {
	client := &rdap.Client{}

	// 1) RDAP 优先
	d, err := client.QueryDomain(domain)
	if err == nil {
		for _, event := range d.Events {
			if strings.EqualFold(event.Action, "expiration") {
				return fmt.Sprintf("%s: RDAP Expiration Date: %s", domain, event.Date)
			}
		}
	}

	// 2) WHOIS
	result, err := whois.Whois(domain)
	if err != nil {
		return fmt.Sprintf("%s 查询失败: WHOIS错误: %v", domain, err)
	}

	// 统一换行
	result = strings.ReplaceAll(result, "\r\n", "\n")

	// 优先字段（更像真正的到期日期）
	keys := []string{
		"Registry Expiry Date:",
		"Registrar Registration Expiration Date:",
		"Expiration Date:",
		"Expiry Date:",
		"expires:",
		"paid-till:",
	}

	for _, raw := range strings.Split(result, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		lower := strings.ToLower(line)

		// 跳过提示/免责声明行
		if strings.HasPrefix(lower, "notice:") ||
			strings.Contains(lower, "terms of use") ||
			strings.Contains(lower, "disclaimer") ||
			strings.Contains(lower, "policy") {
			continue
		}

		for _, k := range keys {
			if strings.HasPrefix(line, k) {
				return fmt.Sprintf("%s: %s", domain, line)
			}
		}
	}

	// 找不到就给个“截断版摘要”，避免 UI/日志把你整段截断得更难看

	snippet := result
	if len(snippet) > 800 {
		snippet = snippet[:800] + " ... (truncated)"
	}
	return fmt.Sprintf("%s: WHOIS未找到明确的到期字段，原文摘要: %s", domain, snippet)
}

func DaysUntilExpiry(expiry string) (int, error) {
	expiryTime, err := time.Parse("2006-01-02", expiry)
	if err != nil {
		return -1, fmt.Errorf("解析到期日期失败: %v", err)
	}
	days := int(time.Until(expiryTime).Hours() / 24)
	return days, nil
}
