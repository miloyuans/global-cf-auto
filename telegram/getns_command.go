package telegram

import (
	"context"
	"fmt"
	"strings"
)

func (h *CommandHandler) handleGetNSCommand(args []string) {
	if len(args) < 1 {
		h.sendText("用法: /getns <domain.com>")
		return
	}
	domain := strings.ToLower(args[0])

	if account, zone, err := h.findZone(domain); err == nil {
		h.sendText(fmt.Sprintf("域名 %s 已在账号 %s 下,无需再次添加到CF,NS: %s", zone.Name, account.Label, strings.Join(zone.NameServers, "\\n")))
		return
	}

	account := h.defaultAccount()
	if account == nil {
		h.sendText("未配置可用的 Cloudflare 账号，无法添加域名。")
		return
	}

	zone, err := h.CFClient.CreateZone(context.Background(), *account, domain)
	if err != nil {
		h.sendText(fmt.Sprintf("添加域名失败: %v,%s---%s", err, domain, account.Label))
		return
	}

	h.sendText(fmt.Sprintf("域名 %s 已经加到账号 %s，NS 请设置为: %s", zone.Name, account.Label, strings.Join(zone.NameServers, "\\n ")))
}
