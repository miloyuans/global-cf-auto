package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"DomainC/cfclient"
)

func (h *CommandHandler) handleSetDNSCommand(args []string) {
	if len(args) < 4 {
		h.sendText("用法: /setdns <domain.com> <type> <name> <content> [proxied:yes/no] [ttl:seconds]\n示例: /setdns example.com A @ 192.0.2.1 yes 3600")
		return
	}
	domain := strings.ToLower(args[0])
	params := cfclient.DNSRecordParams{ // 直接使用 cfclient 包中的类型
		Type:    strings.ToUpper(args[1]),
		Name:    args[2],
		Content: args[3],
		Proxied: false,
		TTL:     1, // Cloudflare 自动 TTL
	}
	if len(args) >= 5 {
		params.Proxied = strings.ToLower(args[4]) == "yes" || strings.ToLower(args[4]) == "true"
	}
	if len(args) >= 6 {
		if ttl, err := strconv.Atoi(args[5]); err == nil && ttl > 0 {
			params.TTL = ttl
		}
	}

	account, _, err := h.findZone(domain)
	if err != nil {
		h.sendText(fmt.Sprintf("域名 %s 不属于任何 Cloudflare 账号。", domain))
		return
	}

	record, err := h.CFClient.UpsertDNSRecord(context.Background(), *account, domain, params)
	if err != nil {
		h.sendText(fmt.Sprintf("设置 DNS 记录失败: %v", err))
		return
	}

	proxyStatus := "否"
	if record.Proxied != nil && *record.Proxied {
		proxyStatus = "是"
	}
	h.sendText(fmt.Sprintf("已在账号 %s 设置记录: %s %s → %s (代理:%s)", account.Label, record.Type, record.Name, record.Content, proxyStatus))
}
