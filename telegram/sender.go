package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Sender 抽象出 Telegram 发送能力，便于替换和测试。
type Sender interface {
	Send(ctx context.Context, msg string) error
	SendWithButtons(ctx context.Context, msg string, buttons [][]Button) error
	StartListener(ctx context.Context, handleCallback func(data string, user *tgbotapi.User), handleMessage func(msg *tgbotapi.Message)) error
	SendDocumentPath(ctx context.Context, filepath string, caption string) error
}

type NoopSender struct{}

func (NoopSender) SendDocumentPath(ctx context.Context, filepath string, caption string) error {
	return nil
}
func (NoopSender) Send(ctx context.Context, msg string) error { return nil }
func (NoopSender) SendWithButtons(ctx context.Context, msg string, buttons [][]Button) error {
	return nil
}
func (NoopSender) StartListener(ctx context.Context, handleCallback func(data string, user *tgbotapi.User), handleMessage func(msg *tgbotapi.Message)) error {
	<-ctx.Done()
	return nil
}

// BotSender 实现了带简单重试和节流的 Telegram 发送能力。
type BotSender struct {
	bot        *tgbotapi.BotAPI
	chatID     int64
	retryTimes int
	rate       *time.Ticker
	timeout    time.Duration
}

func NewBotSender(token string, chatID int64, retryTimes int, rateInterval time.Duration, timeout time.Duration) (*BotSender, error) {
	if token == "" {
		return nil, errors.New("telegram token is empty")
	}
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	sender := &BotSender{
		bot:        bot,
		chatID:     chatID,
		retryTimes: retryTimes,
		rate:       time.NewTicker(rateInterval),
		timeout:    timeout,
	}
	SetDefaultSender(sender)
	return sender, nil
}

const tgMaxLen = 3800

func (s *BotSender) Send(ctx context.Context, msg string) error {
	parts := splitTelegramText(msg, tgMaxLen)
	for i, p := range parts {
		// 可选：给多段加个序号，方便看
		if len(parts) > 1 {
			p = fmt.Sprintf("(%d/%d)\n%s", i+1, len(parts), p)
		}
		if err := s.sendWithMarkup(ctx, tgbotapi.NewMessage(s.chatID, p)); err != nil {
			return err
		}
	}
	return nil
}

func (s *BotSender) SendWithButtons(ctx context.Context, msg string, buttons [][]Button) error {
	message := tgbotapi.NewMessage(s.chatID, msg)
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, r := range buttons {
		var row []tgbotapi.InlineKeyboardButton
		for _, b := range r {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(b.Text, b.CallbackData))
		}
		rows = append(rows, row)
	}
	message.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	return s.sendWithMarkup(ctx, message)
}
func splitTelegramText(s string, limit int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{""}
	}
	if len(s) <= limit {
		return []string{s}
	}

	var out []string
	for len(s) > limit {
		// 1) 优先在 limit 以内找最后一个换行
		cut := strings.LastIndex(s[:limit], "\n")
		// 2) 换行不好用，再找空格
		if cut < limit/3 {
			cut = strings.LastIndex(s[:limit], " ")
		}
		// 3) 还是没有就硬切
		if cut <= 0 {
			cut = limit
		}

		part := strings.TrimSpace(s[:cut])
		if part != "" {
			out = append(out, part)
		}
		s = strings.TrimSpace(s[cut:])
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}
func (s *BotSender) sendWithMarkup(ctx context.Context, msg tgbotapi.MessageConfig) error {

	for attempt := 0; attempt <= s.retryTimes; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.rate.C:
			result := make(chan error, 1)
			sendCtx := ctx
			cancel := func() {}
			if s.timeout > 0 {
				sendCtx, cancel = context.WithTimeout(ctx, s.timeout)
			}

			go func() {
				_, err := s.bot.Send(msg)
				result <- err
			}()

			select {
			case <-sendCtx.Done():
				cancel()
				if attempt == s.retryTimes {
					return fmt.Errorf("发送 Telegram 超时: %w", sendCtx.Err())
				}
				continue
			case err := <-result:
				cancel()
				if err == nil {
					return nil
				}
				if attempt == s.retryTimes {
					return fmt.Errorf("发送 Telegram 失败: %w", err)
				}
				time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			}
		}
	}
	return nil
}

func (s *BotSender) StartListener(ctx context.Context, handleCallback func(data string, user *tgbotapi.User), handleMessage func(msg *tgbotapi.Message)) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := s.bot.GetUpdatesChan(u)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case up := <-updates:
			if up.CallbackQuery != nil && handleCallback != nil {
				handleCallback(up.CallbackQuery.Data, up.CallbackQuery.From)
				cb := tgbotapi.NewCallback(up.CallbackQuery.ID, "操作已收到")
				_, _ = s.bot.Request(cb)
			}
			if up.Message != nil && handleMessage != nil {
				handleMessage(up.Message)
			}
		}
	}
}
func (s *BotSender) SendDocumentPath(ctx context.Context, filepath string, caption string) error {
	if filepath == "" {
		return errors.New("filepath is empty")
	}

	for attempt := 0; attempt <= s.retryTimes; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.rate.C:
			result := make(chan error, 1)

			sendCtx := ctx
			cancel := func() {}
			if s.timeout > 0 {
				sendCtx, cancel = context.WithTimeout(ctx, s.timeout)
			}

			go func() {
				doc := tgbotapi.NewDocument(s.chatID, tgbotapi.FilePath(filepath))
				if caption != "" {
					doc.Caption = caption
				}
				_, err := s.bot.Send(doc)
				result <- err
			}()

			select {
			case <-sendCtx.Done():
				cancel()
				if attempt == s.retryTimes {
					return fmt.Errorf("发送文件超时: %w", sendCtx.Err())
				}
				continue
			case err := <-result:
				cancel()
				if err == nil {
					return nil
				}
				if attempt == s.retryTimes {
					return fmt.Errorf("发送文件失败: %w", err)
				}
				time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			}
		}
	}
	return nil
}
