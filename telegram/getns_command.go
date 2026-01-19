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
	h.setRegistrarNameServers(domain, zone.NameServers)
}

func (h *CommandHandler) setRegistrarNameServers(domain string, nameServers []string) {
	if h.RegistrarManager == nil {
		return
	}
	if len(nameServers) == 0 {
		h.sendText("未获取到 NS，无法同步到注册商。")
		return
	}
	registrar, err := h.RegistrarManager.SetNameServersForDomain(context.Background(), domain, nameServers)
	if err != nil {
		h.sendText(fmt.Sprintf("同步注册商 NS 失败: %v", err))
		return
	}
	h.sendText(fmt.Sprintf("已同步 NS 到注册商账号 %s (%s)。", registrar.Label, registrar.Type))
}
