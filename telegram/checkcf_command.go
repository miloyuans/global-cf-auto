package telegram

import (
	"context"
	"fmt"
	"strings"

	"DomainC/config"
)

func (h *CommandHandler) handleCheckCFCommand(args []string) {
	if len(args) < 1 {
		h.sendText(h.checkCFPromptText())
		return
	}

	selector := strings.TrimSpace(args[0])
	if selector == "" {
		h.sendText(h.checkCFPromptText())
		return
	}

	var targets []config.CF
	if strings.EqualFold(selector, "all") {
		targets = append(targets, h.Accounts...)
		if len(targets) == 0 {
			h.sendText("未配置可用的 Cloudflare 账号，无法检测。")
			return
		}
	} else {
		acc := h.getAccountByLabel(selector)
		if acc == nil {
			h.sendText(fmt.Sprintf("未找到账号 %s。\n\n%s", selector, h.checkCFPromptText()))
			return
		}
		targets = []config.CF{*acc}
	}

	ctx := context.Background()
	var sb strings.Builder
	sb.WriteString("Cloudflare 账号检测结果：\n")

	for _, acc := range targets {
		zones, err := h.CFClient.ListZones(ctx, acc)
		if err != nil {
			h.sendText(fmt.Sprintf("列出账号 %s 的域名失败: %v", acc.Label, err))
			return
		}

		inactive := 0
		sb.WriteString(fmt.Sprintf("\n账号: %s\n", acc.Label))
		for _, zone := range zones {
			if zone.Status == "active" && !zone.Paused {
				continue
			}

			inactive++
			paused := "否"
			if zone.Paused {
				paused = "是"
			}
			sb.WriteString(fmt.Sprintf("- %s (状态: %s, 暂停: %s)\n", zone.Name, zone.Status, paused))
		}

		if inactive == 0 {
			sb.WriteString("- 未发现状态异常的域名\n")
		}
		abuseCount, err := h.CFClient.GetAbuseReportCount(ctx, acc)
		if err != nil {
			sb.WriteString(fmt.Sprintf("- 滥用报告检查失败: %v\n", err))
			continue
		}

		if abuseCount > 0 {
			sb.WriteString(fmt.Sprintf("- 当前账号存在滥用报告: %d\n", abuseCount))
			continue
		}
		sb.WriteString("- 未发现滥用报告\n")
	}

	h.sendText(sb.String())
}

func (h *CommandHandler) checkCFPromptText() string {
	if len(h.Accounts) == 0 {
		return "未配置可用的 Cloudflare 账号，无法检测。"
	}

	var sb strings.Builder
	sb.WriteString("您想检测哪个账号？\n目前可检测的账号：\n")
	for _, a := range h.Accounts {
		if strings.TrimSpace(a.Label) == "" {
			continue
		}
		sb.WriteString("- " + a.Label + "\n")
	}
	sb.WriteString("- all\n\n请输入：\n/checkcf all\n或者：\n/checkcf 账号标签")
	return sb.String()
}
