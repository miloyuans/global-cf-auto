package telegram

import (
	"errors"
	"fmt"
	"strings"

	"DomainC/cfclient"
)

func (h *CommandHandler) handleDeleteCommand(args []string) {
	if len(args) < 1 {
		h.sendText("用法: /delete <domain.com>")
		return
	}
	domain := strings.ToLower(args[0])

	op := formatOperator(h.operator)
	account, _, err := h.findZone(domain)
	if err != nil {
		if errors.Is(err, cfclient.ErrZoneNotFound) {
			h.sendText(fmt.Sprintf("域名 %s 不存在于 Cloudflare。", domain))
			return
		}
		h.sendText(fmt.Sprintf("查询域名失败: %v", err))
		return
	}
	confirmMsg := fmt.Sprintf(
		"⚠️【删除二次确认】\n操作人: %s\n域名: %s\n账号: %s\n\n此操作不可逆，确认要删除该域名（Cloudflare Zone）吗？", op, domain, account.Label,
	)

	buttons := [][]Button{{
		{Text: "✅ 确认删除", CallbackData: fmt.Sprintf("delete_confirm|%s|%s", account.Label, domain)},
		{Text: "❌ 取消", CallbackData: fmt.Sprintf("delete_cancel|%s|%s", account.Label, domain)},
	}}
	SendTelegramAlertWithButtons(confirmMsg, buttons)
}
