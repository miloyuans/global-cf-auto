package telegram

import (
	"context"
	"fmt"
	"net"
	"strings"

	"DomainC/config"
)

func (h *CommandHandler) handleIPListCommand(args []string) {
	if len(args) < 1 {
		h.sendText(h.ipListPromptText())
		return
	}

	selector := strings.TrimSpace(args[0])
	if selector == "" {
		h.sendText(h.ipListPromptText())
		return
	}

	var targets []config.CF
	if strings.EqualFold(selector, "all") {
		targets = append(targets, h.Accounts...)
		if len(targets) == 0 {
			h.sendText("未配置可用的 Cloudflare 账号，无法查询。")
			return
		}
	} else {
		acc := h.getAccountByLabel(selector)
		if acc == nil {
			h.sendText(fmt.Sprintf("未找到账号 %s。\n\n%s", selector, h.ipListPromptText()))
			return
		}
		targets = []config.CF{*acc}
	}

	ctx := context.Background()
	for _, acc := range targets {
		lists, err := h.CFClient.ListCustomLists(ctx, acc)
		if err != nil {
			h.sendText(fmt.Sprintf("查询账号 %s Custom Lists 失败: %v", acc.Label, err))
			continue
		}
		if len(lists) == 0 {
			h.sendText(fmt.Sprintf("账号 %s 暂无 IP Custom Lists。", acc.Label))
			continue
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("【IP Custom Lists】\n账号: %s\n\n", acc.Label))
		for i, list := range lists {
			sb.WriteString(fmt.Sprintf("%d. %s (共 %d 条)\n", i+1, list.Name, list.NumItems))
		}

		var buttons [][]Button
		for _, list := range lists {
			token := SetIPListCallbackPayload(IPListCallbackPayload{
				AccountLabel: acc.Label,
				ListID:       list.ID,
			})
			buttons = append(buttons, []Button{{Text: "编辑 " + list.Name, CallbackData: fmt.Sprintf("iplist_edit|%s", token)}})
		}

		if err := h.Sender.SendWithButtons(ctx, sb.String(), buttons); err != nil {
			h.sendText(fmt.Sprintf("发送账号 %s 列表失败: %v", acc.Label, err))
		}
	}
}

func (h *CommandHandler) ipListPromptText() string {
	if len(h.Accounts) == 0 {
		return "未配置可用的 Cloudflare 账号，无法查询。"
	}

	var sb strings.Builder
	sb.WriteString("您想查询哪个账号的 IP Custom Lists？\n目前可用账号：\n")
	for _, a := range h.Accounts {
		if strings.TrimSpace(a.Label) == "" {
			continue
		}
		sb.WriteString("- " + a.Label + "\n")
	}
	sb.WriteString("- all\n\n请输入：\n/iplist all\n或者：\n/iplist 账号标签")
	return sb.String()
}

func (h *CommandHandler) handlePendingIPListAdd(msgText string, userID int64) bool {
	req, ok := GetPendingIPListAdd(userID)
	if !ok {
		return false
	}

	ip, comment, err := parseIPListInput(msgText)
	if err != nil {
		h.sendText(fmt.Sprintf("IP 格式不正确：%v\n\n请输入：IP 或 CIDR，可选备注。\n示例：\n175.145.84.252/32\n2407:cdc0:b010::/112 备注", err))
		return true
	}

	acc := h.getAccountByLabel(req.AccountLabel)
	if acc == nil {
		ClearPendingIPListAdd(userID)
		h.sendText(fmt.Sprintf("未找到账号 %s，已取消添加。", req.AccountLabel))
		return true
	}

	items, err := h.CFClient.CreateCustomListItem(context.Background(), *acc, req.ListID, ip, comment)
	if err != nil {
		h.sendText(fmt.Sprintf("添加 IP 失败: %v\n请重新输入新的 IP。", err))
		return true
	}
	ClearPendingIPListAdd(userID)

	listName := req.ListID
	if list, err := h.CFClient.GetCustomList(context.Background(), *acc, req.ListID); err == nil && list.Name != "" {
		listName = list.Name
	}

	pages := BuildIPListPages(req.AccountLabel, listName, req.ListID, items, true)
	if err := SendIPListPages(h.Sender, pages); err != nil {
		h.sendText(fmt.Sprintf("发送 IP 列表失败: %v", err))
	}
	return true
}

func parseIPListInput(input string) (string, string, error) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return "", "", fmt.Errorf("输入为空")
	}
	ipStr := strings.TrimSpace(fields[0])
	comment := strings.TrimSpace(strings.Join(fields[1:], " "))

	if strings.Contains(ipStr, "/") {
		if _, _, err := net.ParseCIDR(ipStr); err != nil {
			return "", "", err
		}
		return ipStr, comment, nil
	}
	if net.ParseIP(ipStr) == nil {
		return "", "", fmt.Errorf("IP 无法解析")
	}
	return ipStr, comment, nil
}
