package telegram

import (
	"DomainC/config"
	"context"
	"fmt"
	"strings"
)

func (h *CommandHandler) handleGetNSCommand(args []string) {
	if len(args) < 1 {
		h.sendText("用法: /getns <domain1.com> [domain2.com] ... <accountLabel>")
		return
	}

	if len(h.Accounts) == 0 {
		h.sendText("未配置可用的 Cloudflare 账号，无法添加域名。")
		return
	}

	domains, selected, selectorErr := h.parseGetNSDomainsAndAccount(args)
	if selectorErr != nil {
		h.sendText(selectorErr.Error())
		return
	}

	for _, raw := range domains {
		domain := normalizeDomain(raw)
		if domain == "" {
			continue
		}

		if acc, zone, err := h.findZone(domain); err == nil {
			h.sendText(fmt.Sprintf(
				"域名 %s 已在账号 %s 下, 无需再次添加到CF, NS:\n%s",
				zone.Name,
				acc.Label,
				strings.Join(zone.NameServers, "\n"),
			))
			h.setRegistrarNameServers(domain, zone.NameServers)
			continue
		}

		if selected == nil {
			h.sendText(fmt.Sprintf("域名 %s 不在任何 Cloudflare 账号中。\n%s", domain, h.getNSPromptText()))
			continue
		}

		zone, err := h.CFClient.CreateZone(context.Background(), *selected, domain)
		if err != nil {
			h.sendText(fmt.Sprintf("添加域名失败: %v, %s---%s", err, domain, selected.Label))
			continue
		}

		h.sendText(fmt.Sprintf(
			"域名 %s 已经加到账号 %s，NS 请设置为:\n%s",
			zone.Name,
			selected.Label,
			strings.Join(zone.NameServers, "\n"),
		))

		h.setRegistrarNameServers(domain, zone.NameServers)
	}
}

func (h *CommandHandler) parseGetNSDomainsAndAccount(args []string) ([]string, *config.CF, error) {
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("用法: /getns <domain1.com> [domain2.com] ... <accountLabel>")
	}

	last := strings.TrimSpace(args[len(args)-1])
	if acc := h.getAccountByLabel(last); acc != nil {
		domains := args[:len(args)-1]
		if len(domains) == 0 {
			return nil, nil, fmt.Errorf("请至少提供一个域名。\n%s", h.getNSPromptText())
		}
		return domains, acc, nil
	}

	return args, nil, nil
}

func (h *CommandHandler) getNSPromptText() string {
	if len(h.Accounts) == 0 {
		return "未配置可用的 Cloudflare 账号，无法添加域名。"
	}

	var sb strings.Builder
	sb.WriteString("请选择要添加域名的 Cloudflare 账号（域名若已存在将直接返回 NS）：\n")
	for _, a := range h.Accounts {
		if strings.TrimSpace(a.Label) == "" {
			continue
		}
		sb.WriteString("- " + a.Label + "\n")
	}
	sb.WriteString("\n用法：\n/getns <domain> <账号标签>\n示例：\n/getns example.com acc-a")
	return sb.String()
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
