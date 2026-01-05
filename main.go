package main

import (
	"context"
	"log"
	"time"

	"DomainC/callback"
	"DomainC/cfclient"
	"DomainC/config"
	"DomainC/domain"
	"DomainC/internal/app"
	"DomainC/scheduler"
	"DomainC/telegram"
)

const (
	expiringFile = "expiring_domains.txt"
	failedFile   = "failed_domains.txt"
)

func main() {
	if err := config.Load("config.yaml"); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfClient := cfclient.NewClient()

	var sender telegram.Sender
	botSender, err := telegram.NewBotSender(
		config.Cfg.Telegram.BotToken,
		int64(config.Cfg.Telegram.ChatID),
		2,
		time.Second,
		10*time.Second,
	)
	if err != nil {
		log.Printf("初始化 Telegram 失败，使用空实现: %v", err)
		sender = telegram.NoopSender{}
		telegram.SetDefaultSender(sender)
	} else {
		sender = botSender
	}

	commandHandler := telegram.NewCommandHandler(cfClient, sender, config.Cfg.CloudflareAccounts, int64(config.Cfg.Telegram.ChatID))

	go func() {
		if err := sender.StartListener(ctx, callback.HandleCallback, commandHandler.HandleMessage); err != nil {
			log.Printf("Telegram 监听停止: %v", err)
		}
	}()

	repository := domain.NewFileRepository(config.Cfg.DomainFiles, expiringFile, failedFile)
	service := domain.NewService(cfClient, repository)

	collector := &app.Collector{Service: service, Accounts: config.Cfg.CloudflareAccounts}
	checker := &app.ExpiryCheckerService{
		Whois:        app.DefaultWhoisClient{},
		Repo:         repository,
		AlertWithin:  app.AlertDaysDuration(config.Cfg.AlertDays),
		RateLimit:    time.Second,
		QueryTimeout: 15 * time.Second,
	}
	notifier := &app.NotifierService{Sender: sender, CFClient: cfClient, DeleteTimeout: 10 * time.Second}
	sched := scheduler.NewDailyScheduler()

	application := &app.App{
		Collector: collector,
		Checker:   checker,
		Notifier:  notifier,
		Scheduler: sched,
		AlertHour: 15,
		AlertMin:  0,
	}

	if err := application.Run(ctx); err != nil {
		log.Fatalf("程序退出: %v", err)
	}
}
