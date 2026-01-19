package telegram

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"

	"DomainC/cfclient"

	"github.com/cloudflare/cloudflare-go"
)

func (h *CommandHandler) handleDNSCommand(_ string, args []string) {
	if len(args) < 1 {
		h.sendText("用法: /dns <domain.com | sub.domain.com | URL>")
		return
	}

	raw := strings.TrimSpace(args[0])
	q, err := extractDomainOrHost(raw)
	if err != nil {
		log.Printf("[/dns] invalid input: raw=%q err=%v", raw, err)
		h.sendText(fmt.Sprintf("参数不合法：%v\n用法: /dns <domain.com | sub.domain.com | URL>", err))
		return
	}

	log.Printf("[/dns] query normalized: raw=%q q=%q", raw, q)

	// ✅ findZone 已经被你改成“支持子域候选”的版本（上一轮给你的 findZone 改法）
	account, zone, err := h.findZone(q)
	if err != nil {
		log.Printf("[/dns] findZone failed: q=%q err=%v", q, err)
		if errors.Is(err, cfclient.ErrZoneNotFound) {
			h.sendText(fmt.Sprintf("域名 %s 不属于任何 Cloudflare 账号。", q))
			return
		}
		h.sendText(fmt.Sprintf("查询域名失败: %v", err))
		return
	}

	log.Printf("[/dns] matched: q=%q zone=%q account=%q", q, zone.Name, account.Label)

	records, err := h.CFClient.ListDNSRecords(context.Background(), *account, zone.Name)
	if err != nil {
		log.Printf("[/dns] ListDNSRecords failed: q=%q zone=%q account=%q err=%v", q, zone.Name, account.Label, err)
		h.sendText(fmt.Sprintf("获取 %s 解析失败: %v", q, err))
		return
	}

	if len(records) == 0 {
		log.Printf("[/dns] no records: q=%q zone=%q account=%q", q, zone.Name, account.Label)
		h.sendText(fmt.Sprintf("域名 %s 没有 DNS 记录。", q))
		return
	}

	// ✅ 如果用户查的是子域名，只展示该子域名（以及更深层）的记录；查 zone 本身则展示全 zone。
	filtered := records
	if !strings.EqualFold(q, zone.Name) {
		filtered = filterRecordsByNameOrSubtree(records, q)
	}

	log.Printf("[/dns] records: total=%d filtered=%d q=%q zone=%q", len(records), len(filtered), q, zone.Name)

	if len(filtered) == 0 {
		h.sendText(fmt.Sprintf("在 Zone %s 中没有找到与 %s 相关的 DNS 记录。", zone.Name, q))
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("域名 %s 的 DNS 记录（账号: %s，Zone: %s）：\n", q, account.Label, zone.Name))
	for _, r := range filtered {
		proxied := "否"
		if r.Proxied != nil && *r.Proxied {
			proxied = "是"
		}
		sb.WriteString(fmt.Sprintf("- %s %s → %s (代理: %s, TTL: %d)\n",
			r.Type, r.Name, r.Content, proxied, r.TTL))
	}
	h.sendText(sb.String())
}

// extractDomainOrHost 从用户输入中提取“可用于查 Cloudflare zone 的 host/domain”。
// 支持用户输入:
// - example.com
// - a.example.com
// - https://a.example.com/path
// - a.example.com:8443/path?q=1
// - http://a.example.com:8443
//
// 返回的 q:
// - 小写
// - 去掉末尾点
// - 去掉端口
// - 不包含路径/协议/查询参数
func extractDomainOrHost(input string) (string, error) {
	s := strings.TrimSpace(input)
	s = strings.Trim(s, `"'`) // 去掉用户可能带的引号
	if s == "" {
		return "", fmt.Errorf("empty input")
	}

	// 1) 如果看起来像 URL（带 scheme），优先 url.Parse
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			return "", fmt.Errorf("无法解析 URL: %q", input)
		}
		host := u.Hostname() // 自动去端口，IPv6 也能处理
		return normalizeHost(host)
	}

	// 2) 不带 scheme，但可能是 host/path 或 host:port/path
	//    先去掉 fragment/query
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	//    再只取 path 前面的那段
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("无法从输入中提取域名: %q", input)
	}

	// 3) 如果带端口，拆掉端口
	//    注意：SplitHostPort 要求有端口且 IPv6 要带 []
	//    对于 "a.example.com:8443" 可以直接 split；对于 "a.example.com" 会失败，我们 fallback
	if strings.Contains(s, ":") {
		if host, port, err := net.SplitHostPort(s); err == nil && port != "" {
			return normalizeHost(host)
		}
		// 不是合法的 host:port（也可能只是域名里没有端口但包含 :，比如 IPv6 或者用户乱输）
	}

	return normalizeHost(s)
}

func normalizeHost(host string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimSuffix(h, ".")
	if h == "" {
		return "", fmt.Errorf("empty host")
	}

	// 不支持 IP（Cloudflare zone 是域名，不是 IP）
	if ip := net.ParseIP(h); ip != nil {
		return "", fmt.Errorf("不支持 IP 地址，请输入域名或 URL")
	}

	// 过滤明显不合法的字符（粗过滤：只允许 a-z0-9.-）
	// 更严格可以上正则，但这里先确保不会把路径/奇怪字符带进去
	for _, ch := range h {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '.' || ch == '-' {
			continue
		}
		return "", fmt.Errorf("域名包含非法字符: %q", host)
	}

	// 必须至少包含一个点（例如 example.com）
	if !strings.Contains(h, ".") {
		return "", fmt.Errorf("请输入有效域名（例如 example.com），当前: %q", host)
	}

	return h, nil
}

// filterRecordsByNameOrSubtree：
// - q= a.example.com 时，匹配 a.example.com 以及 *.a.example.com / b.a.example.com ...
func filterRecordsByNameOrSubtree(records []cloudflare.DNSRecord, q string) []cloudflare.DNSRecord {
	q = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(q), "."))
	suffix := "." + q

	out := make([]cloudflare.DNSRecord, 0, len(records))
	for _, r := range records {
		name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.Name), "."))
		if name == q || strings.HasSuffix(name, suffix) {
			out = append(out, r)
		}
	}
	return out
}
