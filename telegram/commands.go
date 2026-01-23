package telegram

import (
	"strings"

	"DomainC/cfclient"
	"DomainC/config"
	"DomainC/registrarclient"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type CommandHandler struct {
	CFClient         cfclient.Client
	RegistrarManager *registrarclient.Manager
	Accounts         []config.CF
	Sender           Sender
	ChatID           int64
	operator         *tgbotapi.User
}

func NewCommandHandler(cf cfclient.Client, registrarManager *registrarclient.Manager, sender Sender, accounts []config.CF, chatID int64) *CommandHandler {
	if cf == nil {
		cf = cfclient.NewClient()
	}
	if sender == nil {
		sender = DefaultSender()
	}
	return &CommandHandler{CFClient: cf, RegistrarManager: registrarManager, Accounts: accounts, Sender: sender, ChatID: chatID}
}

func (h *CommandHandler) HandleMessage(msg *tgbotapi.Message) {
	if msg == nil {
		return
	}
	if h.ChatID != 0 && msg.Chat != nil && msg.Chat.ID != h.ChatID {
		return
	}
	if !msg.IsCommand() {
		if msg.From != nil && msg.Text != "" {
			if h.handlePendingIPListAdd(msg.Text, msg.From.ID) {
				return
			}
		}
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
	case "ssl":
		go h.handleOriginSSLCommand(args)
	case "domainsource":
		go h.handleDomainSourceCommand(args)
	case "cls":
		go h.handleCLSCommand(args)
	case "record":
		go h.handleRecordCommand(args)
	case "checkcf":
		go h.handleCheckCFCommand(args)
	case "deldns":
		go h.handleDelDNSCommand(args)
	case "iplist":
		go h.handleIPListCommand(args)
	}

}
