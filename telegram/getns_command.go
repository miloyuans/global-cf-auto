package telegram

import (
	"context"
	"fmt"
	"strings"
)

func (h *CommandHandler) handleGetNSCommand(args []string) {
	if len(args) < 1 {
		h.sendText("用法: /getns <domain1.com> [domain2.com] [domain3.com] ...")
		return
	}

	account := h.defaultAccount()
	if account == nil {
		h.sendText("未配置可用的 Cloudflare 账号，无法添加域名。")
		return
	}

	// 逐个处理域名（不要并发，方便日志/输出有序，也避免 CF/注册商限速）
	for _, raw := range args {
		domain := normalizeDomain(raw)
		if domain == "" {
			continue
		}

		// 1) 先找是否已经在 CF
		if acc, zone, err := h.findZone(domain); err == nil {
			h.sendText(fmt.Sprintf(
				"域名 %s 已在账号 %s 下, 无需再次添加到CF, NS:\n%s",
				zone.Name,
				acc.Label,
				strings.Join(zone.NameServers, "\n"),
			))

			// 已存在也尝试同步注册商（可选：如果你不想同步，把下面这行删掉即可）
			h.setRegistrarNameServers(domain, zone.NameServers)
			continue
		}

		// 2) 不存在则创建 Zone
		zone, err := h.CFClient.CreateZone(context.Background(), *account, domain)
		if err != nil {
			h.sendText(fmt.Sprintf("添加域名失败: %v, %s---%s", err, domain, account.Label))
			continue
		}

		h.sendText(fmt.Sprintf(
			"域名 %s 已经加到账号 %s，NS 请设置为:\n%s",
			zone.Name,
			account.Label,
			strings.Join(zone.NameServers, "\n"),
		))

		// 3) 同步到注册商（注册商可能不同：由 RegistrarManager 根据 domain 自己路由）
		h.setRegistrarNameServers(domain, zone.NameServers)
	}
}

func normalizeDomain(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	// 允许用户输入带逗号/分号（比如复制粘贴 "a.com,b.com"）
	s = strings.Trim(s, ",;")
	return s
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
