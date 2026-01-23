package callback

import (
	"context"
	"fmt"
	"log"
	"strings"

	"DomainC/cfclient"
	"DomainC/telegram"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// HandleCallback 处理来自 Notifier 的内联按钮回调
// callbackData 格式：action|accountLabel|domain|[paused yes/no]
func HandleCallback(cb *tgbotapi.CallbackQuery) {
	callbackData := cb.Data
	user := cb.From
	parts := strings.Split(callbackData, "|")
	if len(parts) < 1 {
		log.Printf("无效的回调数据: %s", callbackData)
		return
	}

	action := parts[0]

	// 避免“处理中”按钮再触发一堆日志
	if action == "noop" {
		return
	}
	if strings.HasPrefix(action, "iplist_") {
		handleIPListCallback(action, parts, user, cb)
		return
	}
	if len(parts) < 3 {
		log.Printf("无效的回调数据: %s", callbackData)
		return
	}

	accountLabel := parts[1]
	domain := strings.ToLower(parts[2])

	paused := ""
	if len(parts) >= 4 {
		paused = parts[3]
	}

	log.Printf("处理回调: action=%s, account=%s, domain=%s, user=%s", action, accountLabel, domain, user.UserName)

	// 创建统一的 Cloudflare 客户端实例
	client := cfclient.NewClient()

	account := cfclient.GetAccountByLabel(accountLabel)
	if account == nil {
		log.Printf("未找到账号标签: %s", accountLabel)
		telegram.SendTelegramAlert(fmt.Sprintf("操作失败：未找到账号 %s", accountLabel))
		return
	}

	switch action {
	case "pause":
		go func() {
			var successMsg, failMsg string
			if paused == "yes" {
				successMsg = fmt.Sprintf("%s 禁用域名成功: %s --- %s", user.UserName, domain, accountLabel)
				failMsg = fmt.Sprintf("%s 禁用域名失败: %s --- %s (%%v)", user.UserName, domain, accountLabel)
			} else {
				successMsg = fmt.Sprintf("%s 解除禁用成功: %s --- %s", user.UserName, domain, accountLabel)
				failMsg = fmt.Sprintf("%s 解除禁用失败: %s --- %s (%%v)", user.UserName, domain, accountLabel)
			}

			err := client.PauseDomain(context.Background(), *account, domain, paused == "yes")
			if err != nil {
				telegram.SendTelegramAlert(fmt.Sprintf(failMsg, err))
			} else {
				telegram.SendTelegramAlert(successMsg)
			}
		}()

	case "DNS":
		go func() {
			records, err := client.ListDNSRecords(context.Background(), *account, domain)
			if err != nil {
				telegram.SendTelegramAlert(fmt.Sprintf("查询域名解析失败: %s --- %s (%v)", domain, accountLabel, err))
				return
			}

			if len(records) == 0 {
				telegram.SendTelegramAlert(fmt.Sprintf("域名 %s --- %s 没有任何解析记录。", domain, accountLabel))
				return
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("【域名解析记录】\n域名: %s\n来源: %s\n\n", domain, accountLabel))

			for _, r := range records {
				proxied := "关闭"
				if r.Proxied != nil && *r.Proxied {
					proxied = "开启"
				}
				sb.WriteString(fmt.Sprintf("%s %s → %s (代理: %s)\n", r.Type, r.Name, r.Content, proxied))
			}

			telegram.SendTelegramAlert(sb.String())
		}()

	case "delete":
		go func() {
			confirmMsg := fmt.Sprintf(
				"⚠️【删除二次确认】\n操作人: %s\n域名: %s\n账号: %s\n\n此操作不可逆，确认要删除该域名（Cloudflare Zone）吗？",
				user.UserName, domain, accountLabel,
			)

			buttons := [][]telegram.Button{{
				{Text: "✅ 确认删除", CallbackData: fmt.Sprintf("delete_confirm|%s|%s", accountLabel, domain)},
				{Text: "❌ 取消", CallbackData: fmt.Sprintf("delete_cancel|%s|%s", accountLabel, domain)},
			}}

			telegram.SendTelegramAlertWithButtons(confirmMsg, buttons)
		}()

	case "delete_confirm":
		go func() {
			err := client.DeleteDomain(context.Background(), *account, domain)
			if err != nil {
				telegram.SendTelegramAlert(fmt.Sprintf("删除域名失败: %s --- %s (%v)", domain, accountLabel, err))
				return
			}
			telegram.SendTelegramAlert(fmt.Sprintf("✅ 删除域名成功: %s --- %s (操作人: %s)", domain, accountLabel, user.UserName))
		}()

	case "delete_cancel":
		go func() {
			telegram.SendTelegramAlert(fmt.Sprintf("已取消删除: %s --- %s (操作人: %s)", domain, accountLabel, user.UserName))
		}()
	}
}
func handleIPListCallback(action string, parts []string, user *tgbotapi.User, cb *tgbotapi.CallbackQuery) {
	if len(parts) < 2 {
		log.Printf("无效的 iplist 回调数据: %v", parts)
		return
	}
	token := parts[1]
	payload, ok := telegram.GetIPListCallbackPayload(token)
	if !ok {
		telegram.SendTelegramAlert("操作已过期，请重新打开列表。")
		return
	}

	accountLabel := payload.AccountLabel
	account := cfclient.GetAccountByLabel(accountLabel)
	if account == nil {
		log.Printf("未找到账号标签: %s", accountLabel)
		telegram.SendTelegramAlert(fmt.Sprintf("操作失败：未找到账号 %s", accountLabel))
		return
	}

	client := cfclient.NewClient()
	sender := telegram.DefaultSender()
	switch action {
	case "iplist_edit":
		listID := payload.ListID
		go func() {
			list, err := client.GetCustomList(context.Background(), *account, listID)
			if err != nil {
				telegram.SendTelegramAlert(fmt.Sprintf("获取 Custom List 失败: %v", err))
				return
			}
			items, err := client.ListCustomListItems(context.Background(), *account, listID)
			if err != nil {
				telegram.SendTelegramAlert(fmt.Sprintf("获取 Custom List 条目失败: %v", err))
				return
			}
			pages := telegram.BuildIPListPages(accountLabel, list.Name, listID, items, true)
			for _, page := range pages {
				if err := sender.SendWithButtons(context.Background(), page.Message, page.Buttons); err != nil {
					log.Printf("发送 IP 列表失败: %v", err)
				}
			}
		}()

	case "iplist_delete":
		listID := payload.ListID
		itemID := payload.ItemID
		if itemID == "" {
			telegram.SendTelegramAlert("删除失败：缺少条目 ID。")
			return
		}

		go func() {
			listName := listID
			if list, err := client.GetCustomList(context.Background(), *account, listID); err == nil && list.Name != "" {
				listName = list.Name
			}

			confirmMsg := fmt.Sprintf(
				"⚠️【删除 IP 二次确认】\n操作人: %s\n账号: %s\n列表: %s\n条目ID: %s\n\n此操作不可逆，确认要删除该条目吗？",
				user.UserName, accountLabel, listName, itemID,
			)

			buttons := [][]telegram.Button{{
				{Text: "✅ 确认删除", CallbackData: fmt.Sprintf("iplist_confirm|%s", token)},
				{Text: "❌ 取消", CallbackData: fmt.Sprintf("iplist_cancel|%s", token)},
			}}

			telegram.SendTelegramAlertWithButtons(confirmMsg, buttons)
		}()
	case "iplist_confirm":

		if cb.Message != nil {
			_ = sender.EditButtons(context.Background(),
				cb.Message.Chat.ID,
				cb.Message.MessageID,
				[][]telegram.Button{{
					{Text: "✅ 已确认，处理中…", CallbackData: "noop"},
				}},
			)
		}

		listID := payload.ListID
		itemID := payload.ItemID
		if itemID == "" {
			telegram.SendTelegramAlert("删除失败：缺少条目 ID。")
			return
		}

		go func() {
			items, err := client.DeleteCustomListItem(context.Background(), *account, listID, itemID)
			if err != nil {
				telegram.SendTelegramAlert(fmt.Sprintf("删除 IP 失败: %v", err))
				return
			}

			telegram.SendTelegramAlert(fmt.Sprintf("✅ 删除 IP 成功（操作人: %s）", user.UserName))

			if cb.Message != nil {
				_ = sender.EditButtons(context.Background(),
					cb.Message.Chat.ID,
					cb.Message.MessageID,
					[][]telegram.Button{{
						{Text: "✅ 已完成", CallbackData: "noop"},
					}},
				)

				_ = sender.ClearButtons(context.Background(), cb.Message.Chat.ID, cb.Message.MessageID)
			}

			listName := listID
			if list, err := client.GetCustomList(context.Background(), *account, listID); err == nil && list.Name != "" {
				listName = list.Name
			}
			pages := telegram.BuildIPListPages(accountLabel, listName, listID, items, true)
			for _, page := range pages {
				if err := sender.SendWithButtons(context.Background(), page.Message, page.Buttons); err != nil {
					log.Printf("发送 IP 列表失败: %v", err)
				}
			}
		}()

	case "iplist_cancel":
		go func() {
			telegram.SendTelegramAlert(fmt.Sprintf("已取消删除（操作人: %s）", user.UserName))
		}()
	case "iplist_add":
		listID := payload.ListID
		go func() {
			telegram.SetPendingIPListAdd(user.ID, telegram.IPListAddRequest{
				AccountLabel: accountLabel,
				ListID:       listID,
			})
			telegram.SendTelegramAlert("请输入要添加的 IP 地址（支持 IPv4/IPv6/CIDR）。\n可选备注：在 IP 后用空格输入。\n示例：\n175.145.84.252/32\n2407:cdc0:b010::/112 备注")
		}()
	}
}
