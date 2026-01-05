package telegram

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"DomainC/cfclient"
	"DomainC/config"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (h *CommandHandler) findZone(domain string) (*config.CF, cfclient.ZoneDetail, error) {
	var lastErr error
	for i := range h.Accounts {
		acc := h.Accounts[i]
		zone, err := h.CFClient.GetZoneDetails(context.Background(), acc, domain)
		if err != nil {
			if errors.Is(err, cfclient.ErrZoneNotFound) {
				lastErr = err
				continue
			}
			return nil, cfclient.ZoneDetail{}, err
		}
		return &acc, zone, nil
	}
	if lastErr == nil {
		lastErr = cfclient.ErrZoneNotFound
	}
	return nil, cfclient.ZoneDetail{}, lastErr
}

func (h *CommandHandler) deleteZone(domain string) (*config.CF, error) {
	var lastErr error
	for i := range h.Accounts {
		acc := h.Accounts[i]
		err := h.CFClient.DeleteDomain(context.Background(), acc, domain)
		if err != nil {
			if errors.Is(err, cfclient.ErrZoneNotFound) {
				lastErr = err
				continue
			}
			return nil, err
		}
		return &acc, nil
	}
	if lastErr == nil {
		lastErr = cfclient.ErrZoneNotFound
	}
	return nil, lastErr
}

// defaultAccount 随机返回一个 Cloudflare 账号配置
func (h *CommandHandler) defaultAccount() *config.CF {
	if len(h.Accounts) == 0 {
		return nil
	}
	idx := rand.Intn(len(h.Accounts))
	return &h.Accounts[idx]
}

func (h *CommandHandler) sendText(msg string) {
	_ = h.Sender.Send(context.Background(), msg)
}

func formatOperator(u *tgbotapi.User) string {
	if u == nil {
		return "unknown"
	}
	if u.UserName != "" {
		return "@" + u.UserName
	}
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name != "" {
		return name
	}
	return fmt.Sprintf("id:%d", u.ID)
}

func deriveDomainFromName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return ""
}
