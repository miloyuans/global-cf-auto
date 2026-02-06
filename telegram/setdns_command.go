package telegram

import (
	"context"
	"fmt"
	"strings"

	"DomainC/cfclient"
)

func (h *CommandHandler) handleSetDNSCommand(args []string) {
	if len(args) < 4 {
		h.sendText("用法: /setdns <domain.com> <type> <name> <content> [proxied:yes/no]\n示例: /setdns example.com A @ 192.0.2.1 yes")
		return
	}

	domain := strings.ToLower(strings.TrimSpace(args[0]))

	// 主记录参数
	params := cfclient.DNSRecordParams{
		Type:    strings.ToUpper(args[1]),
		Name:    args[2],
		Content: args[3],
		Proxied: false,
		TTL:     3600, // 固定默认 3600
	}

	// 可选 proxied
	if len(args) >= 5 {
		v := strings.ToLower(strings.TrimSpace(args[4]))
		params.Proxied = v == "yes" || v == "true" || v == "1"
	}

	account, _, err := h.findZone(domain)
	if err != nil {
		h.sendText(fmt.Sprintf("域名 %s 不属于任何 Cloudflare 账号。", domain))
		return
	}

	// 1) upsert 主记录
	record, err := h.CFClient.UpsertDNSRecord(context.Background(), *account, domain, params)
	if err != nil {
		h.sendText(fmt.Sprintf("设置 DNS 记录失败: %v", err))
		return
	}

	proxyStatus := "否"
	if record.Proxied != nil && *record.Proxied {
		proxyStatus = "是"
	}
	h.sendText(fmt.Sprintf("已在账号 %s 设置记录: %s %s → %s (代理:%s, TTL:%d)",
		account.Label, record.Type, record.Name, record.Content, proxyStatus, params.TTL,
	))

	// 2) 如果用户设置的是根域(@)，顺便把 www 也解析掉：www.<domain> CNAME <domain>
	if strings.TrimSpace(params.Name) == "@" {
		wwwParams := cfclient.DNSRecordParams{
			Type:    "CNAME",
			Name:    "www",
			Content: domain,         // 指向根域 domain.com
			Proxied: params.Proxied, // 跟随用户 proxied（你也可改成固定值）
			TTL:     3600,
		}

		wwwRecord, wwwErr := h.CFClient.UpsertDNSRecord(context.Background(), *account, domain, wwwParams)
		if wwwErr != nil {
			h.sendText(fmt.Sprintf("已设置根域记录，但设置 www CNAME 失败: %v", wwwErr))
			return
		}

		wwwProxyStatus := "否"
		if wwwRecord.Proxied != nil && *wwwRecord.Proxied {
			wwwProxyStatus = "是"
		}
		h.sendText(fmt.Sprintf("已自动设置 www 记录: %s %s → %s (代理:%s, TTL:%d)",
			wwwRecord.Type, wwwRecord.Name, wwwRecord.Content, wwwProxyStatus, wwwParams.TTL,
		))
	}
}
