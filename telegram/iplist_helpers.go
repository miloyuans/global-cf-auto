package telegram

import (
	"context"
	"fmt"
	"strings"

	cloudflare "github.com/cloudflare/cloudflare-go"
)

const ipListItemsPerPage = 20

type IPListPage struct {
	Message string
	Buttons [][]Button
}

func BuildIPListPages(accountLabel string, listName string, listID string, items []cloudflare.ListItem, includeAdd bool) []IPListPage {
	header := fmt.Sprintf("【IP Custom List】\n账号: %s\n列表: %s\n\n", accountLabel, listName)
	if listName == "" {
		header = fmt.Sprintf("【IP Custom List】\n账号: %s\n列表: %s\n\n", accountLabel, listID)
	}

	if len(items) == 0 {
		buttons := [][]Button{}
		if includeAdd {
			token := SetIPListCallbackPayload(IPListCallbackPayload{
				AccountLabel: accountLabel,
				ListID:       listID,
			})
			buttons = append(buttons, []Button{{Text: "添加", CallbackData: fmt.Sprintf("iplist_add|%s", token)}})
		}
		return []IPListPage{{
			Message: header + "暂无 IP 记录。",
			Buttons: buttons,
		}}
	}

	var pages []IPListPage
	for start := 0; start < len(items); start += ipListItemsPerPage {
		end := start + ipListItemsPerPage
		if end > len(items) {
			end = len(items)
		}

		var sb strings.Builder
		sb.WriteString(header)
		for i := start; i < end; i++ {
			item := items[i]
			ip := "-"
			if item.IP != nil {
				ip = *item.IP
			}
			comment := strings.TrimSpace(item.Comment)
			if comment == "" {
				comment = "无"
			}
			sb.WriteString(fmt.Sprintf("%d. %s | 备注: %s\n", i+1, ip, comment))
		}

		var buttons [][]Button
		for i := start; i < end; i++ {
			item := items[i]
			ip := "IP"
			if item.IP != nil && *item.IP != "" {
				ip = *item.IP
			}
			if item.ID == "" {
				continue
			}
			token := SetIPListCallbackPayload(IPListCallbackPayload{
				AccountLabel: accountLabel,
				ListID:       listID,
				ItemID:       item.ID,
			})
			buttons = append(buttons, []Button{{Text: "删除 " + ip, CallbackData: fmt.Sprintf("iplist_delete|%s", token)}})
		}
		if includeAdd && end == len(items) {
			token := SetIPListCallbackPayload(IPListCallbackPayload{
				AccountLabel: accountLabel,
				ListID:       listID,
			})
			buttons = append(buttons, []Button{{Text: "添加", CallbackData: fmt.Sprintf("iplist_add|%s", token)}})
		}
		pages = append(pages, IPListPage{
			Message: sb.String(),
			Buttons: buttons,
		})
	}

	return pages
}

func SendIPListPages(sender Sender, pages []IPListPage) error {
	for _, page := range pages {
		if err := sender.SendWithButtons(context.Background(), page.Message, page.Buttons); err != nil {
			return err
		}
	}
	return nil
}
