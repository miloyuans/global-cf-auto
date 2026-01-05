package telegram

import (
	"strings"

	"DomainC/cfclient"
	"DomainC/config"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// CommandHandler 处理群组中的命令消息
// 需要传入 Cloudflare 客户端与账号列表。
type CommandHandler struct {
	CFClient cfclient.Client
	Accounts []config.CF
	Sender   Sender
	ChatID   int64
	operator *tgbotapi.User
}

func NewCommandHandler(cf cfclient.Client, sender Sender, accounts []config.CF, chatID int64) *CommandHandler {
	if cf == nil {
		cf = cfclient.NewClient()
	}
	if sender == nil {
		sender = DefaultSender()
	}
	return &CommandHandler{CFClient: cf, Accounts: accounts, Sender: sender, ChatID: chatID}
}

func (h *CommandHandler) HandleMessage(msg *tgbotapi.Message) {
	if msg == nil {
		return
	}
	if h.ChatID != 0 && msg.Chat != nil && msg.Chat.ID != h.ChatID {
		return
	}
	if !msg.IsCommand() {
		return
	}
	h.operator = msg.From
	args := strings.Fields(msg.CommandArguments())
	switch msg.Command() {
	case "dns":
		go h.handleDNSCommand(strings.ToLower(msg.Command()), args)
	case "getns":
		go h.handleGetNSCommand(args)
	case "status":
		go h.handleStatusCommand(args)
	case "delete":
		go h.handleDeleteCommand(args)
	case "setdns":
		go h.handleSetDNSCommand(args)
	case "csv":
		go h.handleCSVCommand(args)
	case "originssl":
		go h.handleOriginSSLCommand(args)
	}
}
