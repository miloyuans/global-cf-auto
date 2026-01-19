package telegram

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"DomainC/config"
)

func (h *CommandHandler) handleDomainSourceCommand(args []string) {
	if h.RegistrarManager == nil {
		h.sendText("未配置注册商客户端。")
		return
	}
	registrars := h.RegistrarManager.Registrars()
	if len(registrars) == 0 {
		h.sendText("未配置可用的注册商账号。")
		return
	}
	if len(args) == 0 {
		h.sendText(h.domainSourcePromptText(registrars))
		return
	}
	target := strings.TrimSpace(args[0])
	if target == "" {
		h.sendText(h.domainSourcePromptText(registrars))
		return
	}
	if strings.EqualFold(target, "all") {
		for _, registrar := range registrars {
			h.sendDomainsForRegistrar(registrar)
		}
		return
	}
	registrar, ok := h.RegistrarManager.RegistrarByLabel(target)
	if !ok {
		h.sendText(h.domainSourcePromptText(registrars))
		return
	}
	h.sendDomainsForRegistrar(registrar)
}

func (h *CommandHandler) domainSourcePromptText(registrars []config.Registrar) string {
	labels := make([]string, 0, len(registrars))
	for _, r := range registrars {
		labels = append(labels, r.Label)
	}
	sort.Strings(labels)
	return fmt.Sprintf("用法: /domainsource <label|all>\n可查询账号\n: %s", strings.Join(labels, "\n "))
}

func (h *CommandHandler) sendDomainsForRegistrar(registrar config.Registrar) {
	domains, err := h.RegistrarManager.ListDomainsForRegistrar(context.Background(), registrar)
	if err != nil {
		h.sendText(fmt.Sprintf("查询账号 %s(%s) 域名失败: %v", registrar.Label, registrar.Type, err))
		return
	}
	sort.Strings(domains)
	h.sendText(fmt.Sprintf("账号 %s(%s) 域名列表（%d）:\n%s", registrar.Label, registrar.Type, len(domains), strings.Join(domains, "\n")))
}
