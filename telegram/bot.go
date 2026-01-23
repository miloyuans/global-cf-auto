package telegram

import (
	"context"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var defaultSender Sender = NoopSender{}

type Button struct {
	Text         string
	CallbackData string
}

func SetDefaultSender(sender Sender) {
	if sender != nil {
		defaultSender = sender
	}
}

func DefaultSender() Sender {
	return defaultSender
}

func SendTelegramAlert(msg string) {
	if err := defaultSender.Send(context.Background(), msg); err != nil {
		log.Printf("发送 Telegram 消息失败: %v", err)
	}
}

func SendTelegramAlertWithButtons(msg string, buttons [][]Button) {
	if err := defaultSender.SendWithButtons(context.Background(), msg, buttons); err != nil {
		log.Printf("发送 Telegram 按钮消息失败: %v", err)
	}
}

func StartListener(handleCallback func(cb *tgbotapi.CallbackQuery), handleMessage func(msg *tgbotapi.Message)) {
	go func() {
		if err := defaultSender.StartListener(context.Background(), handleCallback, handleMessage); err != nil {
			log.Printf("Telegram 监听异常: %v", err)
		}
	}()
}
