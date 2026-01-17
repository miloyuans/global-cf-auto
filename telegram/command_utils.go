package telegram

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"

	"DomainC/cfclient"
	"DomainC/config"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (h *CommandHandler) findZone(domain string) (*config.CF, cfclient.ZoneDetail, error) {
	// 0) normalize（和你 ListDNSRecords 同样逻辑，避免不一致）
	orig := domain
	domain = strings.TrimSpace(strings.ToLower(domain))
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	if i := strings.IndexByte(domain, '/'); i >= 0 {
		domain = domain[:i]
	}
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		log.Printf("[findZone] empty domain after normalize, orig=%q", orig)
		return nil, cfclient.ZoneDetail{}, cfclient.ErrZoneNotFound
	}

	// 1) 候选 zone：支持 a.b.example.com -> a.b.example.com, b.example.com, example.com
	parts := strings.Split(domain, ".")
	cands := []string{domain}
	if len(parts) >= 2 {
		cands = cands[:0]
		for i := 0; i <= len(parts)-2; i++ {
			cands = append(cands, strings.Join(parts[i:], "."))
		}
	}

	log.Printf("[findZone] start: orig=%q normalized=%q cands=%v", orig, domain, cands)

	var lastErr error

	// 2) 先按候选逐个尝试（每个候选遍历所有账号）
	for _, cand := range cands {
		for i := range h.Accounts {
			acc := h.Accounts[i]
			log.Printf("[findZone] try: cand=%q account=%q", cand, acc.Label)

			zone, err := h.CFClient.GetZoneDetails(context.Background(), acc, cand)
			if err != nil {
				if errors.Is(err, cfclient.ErrZoneNotFound) {
					lastErr = err
					continue
				}
				log.Printf("[findZone] GetZoneDetails error: cand=%q account=%q err=%v", cand, acc.Label, err)
				return nil, cfclient.ZoneDetail{}, err
			}

			log.Printf("[findZone] matched: cand=%q zone=%q account=%q", cand, zone.Name, acc.Label)
			return &acc, zone, nil
		}
	}

	if lastErr == nil {
		lastErr = cfclient.ErrZoneNotFound
	}
	log.Printf("[findZone] not found: normalized=%q lastErr=%v", domain, lastErr)
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

func (h *CommandHandler) defaultAccount() *config.CF {
	n := len(h.Accounts)
	if n == 0 {
		return nil
	}

	if n >= 2 {
		idx := rand.Intn(2)
		return &h.Accounts[idx]
	}
	return &h.Accounts[0]
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
