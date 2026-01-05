package telegram

import (
	"errors"
	"fmt"
	"strings"

	"DomainC/cfclient"
)

func (h *CommandHandler) handleStatusCommand(args []string) {
	if len(args) < 1 {
		h.sendText("用法: /status <domain.com>")
		return
	}
	domain := strings.ToLower(args[0])

	account, zone, err := h.findZone(domain)
	if err != nil {
		if errors.Is(err, cfclient.ErrZoneNotFound) {
			h.sendText(fmt.Sprintf("域名 %s 不属于任何 Cloudflare 账号。", domain))
			return
		}
		h.sendText(fmt.Sprintf("查询状态失败: %v", err))
		return
	}

	operator := formatOperator(h.operator)
	h.sendText(fmt.Sprintf("域名 %s 状态: %s (暂停: %v)\n账号: %s\n操作人: %s", zone.Name, zone.Status, zone.Paused, account.Label, operator))
}
