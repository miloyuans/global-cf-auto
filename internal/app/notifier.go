package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"DomainC/cfclient"
	"DomainC/config"
	"DomainC/domain"
	"DomainC/telegram"
	"DomainC/tools"
)

type NotifierService struct {
	Sender        telegram.Sender
	CFClient      cfclient.Client
	DeleteTimeout time.Duration
}

func (n *NotifierService) Notify(ctx context.Context, domains []domain.DomainSource) error {
	if n.Sender == nil {
		return ErrMissingDependencies
	}
	for _, ds := range domains {
		days, err := tools.DaysUntilExpiry(ds.Expiry)
		if err != nil {
			log.Printf("无法计算剩余天数: %v", err)
			continue
		}

		if ds.IsCF {
			n.notifyCloudflare(ctx, ds, days)
			continue
		}

		msg := fmt.Sprintf(
			"【域名即将到期】\n域名: %s\n来源: %s\n到期时间: %s\n非CF账户的域名请手工处理。",
			ds.Domain,
			ds.Source,
			ds.Expiry,
		)
		if err := n.Sender.Send(ctx, msg); err != nil {
			log.Printf("发送非CF域名提醒失败: %v", err)
		}
	}
	return nil
}
func (n *NotifierService) NotifyFailures(ctx context.Context, failures []domain.FailureRecord) error {
	if n.Sender == nil {
		return ErrMissingDependencies
	}
	if len(failures) == 0 {
		return nil
	}

	var builder strings.Builder
	builder.WriteString("【以下域名未能从rdap及whois获取到期时间】\n")
	builder.WriteString("请手动检查并处理：\n\n")
	for _, f := range failures {
		builder.WriteString(fmt.Sprintf("- %s (来源: %s): %s\n", f.Domain, f.Source, f.Reason))
	}

	return n.Sender.Send(ctx, builder.String())
}
func (n *NotifierService) notifyCloudflare(ctx context.Context, ds domain.DomainSource, days int) {
	msg := fmt.Sprintf(
		"【域名即将到期】\n域名: %s\n来源: %s\n到期时间: %s\n注意：如果没人响应，遇到到期后将自动从CF删除",
		ds.Domain,
		ds.Source,
		ds.Expiry,
	)
	buttons := [][]telegram.Button{{
		{Text: "暂停域名", CallbackData: fmt.Sprintf("pause|%s|%s|yes", ds.Source, ds.Domain)},
		{Text: "恢复暂停", CallbackData: fmt.Sprintf("pause|%s|%s|no", ds.Source, ds.Domain)},
		{Text: "查询解析", CallbackData: fmt.Sprintf("DNS|%s|%s", ds.Source, ds.Domain)},
		{Text: "删除域名", CallbackData: fmt.Sprintf("delete|%s|%s", ds.Source, ds.Domain)},
	}}
	if err := n.Sender.SendWithButtons(ctx, msg, buttons); err != nil {
		log.Printf("发送 CF 域名提醒失败: %v", err)
	}

	if days == 1 && n.CFClient != nil {
		account := cfclient.GetAccountByLabel(ds.Source)
		if account == nil {
			log.Printf("未找到账号: %s", ds.Source)
			return
		}
		deleteCtx := ctx
		cancel := func() {}
		if n.DeleteTimeout > 0 {
			deleteCtx, cancel = context.WithTimeout(ctx, n.DeleteTimeout)
		}
		go func(acc config.CF, domain string) {
			defer cancel()
			if err := n.CFClient.DeleteDomain(deleteCtx, acc, domain); err != nil {
				_ = n.Sender.Send(ctx, fmt.Sprintf("⚠️ 自动删除域名失败: %s (%v)", domain, err))
				return
			}
			_ = n.Sender.Send(ctx, fmt.Sprintf("✅ 已自动删除即将到期的域名: %s", domain))
		}(*account, ds.Domain)
	}
}
