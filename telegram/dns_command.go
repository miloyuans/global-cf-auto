package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"DomainC/cfclient"
)

func (h *CommandHandler) handleDNSCommand(_ string, args []string) {
	if len(args) < 1 {
		h.sendText("用法: /dns <domain.com>")
		return
	}
	domain := strings.ToLower(args[0])

	account, zone, err := h.findZone(domain)
	if err != nil {
		if errors.Is(err, cfclient.ErrZoneNotFound) {
			h.sendText(fmt.Sprintf("域名 %s 不属于任何 Cloudflare 账号。", domain))
			return
		}
		h.sendText(fmt.Sprintf("查询域名失败: %v", err))
		return
	}

	records, err := h.CFClient.ListDNSRecords(context.Background(), *account, zone.Name)
	if err != nil {
		h.sendText(fmt.Sprintf("获取 %s 解析失败: %v", domain, err))
		return
	}

	if len(records) == 0 {
		h.sendText(fmt.Sprintf("域名 %s 没有 DNS 记录。", domain))
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("域名 %s 的 DNS 记录（账号: %s）：\n", domain, account.Label))
	for _, r := range records {
		proxied := "否"
		if *r.Proxied {
			proxied = "是"
		}
		sb.WriteString(fmt.Sprintf("- %s %s → %s (代理: %s, TTL: %d)\n", r.Type, r.Name, r.Content, proxied, r.TTL))
	}
	h.sendText(sb.String())
}
