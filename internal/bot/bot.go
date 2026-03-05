package bot

import (
	"context"
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/config"
	"github.com/sgx-labs/same-telegram/internal/notify"
)

// Bot wraps the Telegram bot API with SAME-specific functionality.
type Bot struct {
	api    *tgbotapi.BotAPI
	cfg    *config.Config
	logger *log.Logger

	// allowedUsers is a set of allowed Telegram user IDs for fast lookup.
	allowedUsers map[int64]bool
}

// New creates a new Bot instance.
func New(cfg *config.Config, logger *log.Logger) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.Bot.Token)
	if err != nil {
		return nil, fmt.Errorf("create bot API: %w", err)
	}

	allowed := make(map[int64]bool, len(cfg.Bot.AllowedUserIDs))
	for _, id := range cfg.Bot.AllowedUserIDs {
		allowed[id] = true
	}

	logger.Printf("Authorized on Telegram as @%s", api.Self.UserName)

	return &Bot{
		api:          api,
		cfg:          cfg,
		logger:       logger,
		allowedUsers: allowed,
	}, nil
}

// Run starts the bot polling loop. Blocks until context is cancelled.
func (b *Bot) Run(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			return ctx.Err()

		case update := <-updates:
			if update.Message != nil {
				b.handleUpdate(update.Message)
			}
			if update.CallbackQuery != nil {
				b.handleCallback(update.CallbackQuery)
			}
		}
	}
}

// SendNotification sends a notification message to all allowed users.
func (b *Bot) SendNotification(n *notify.Notification) {
	text, keyboard := FormatNotification(n)

	for userID := range b.allowedUsers {
		msg := tgbotapi.NewMessage(userID, text)
		msg.ParseMode = "Markdown"
		if keyboard != nil {
			msg.ReplyMarkup = keyboard
		}
		if _, err := b.api.Send(msg); err != nil {
			b.logger.Printf("Failed to send notification to %d: %v", userID, err)
		}
	}
}

func (b *Bot) handleUpdate(msg *tgbotapi.Message) {
	// Security: silently drop messages from unknown users
	if !b.allowedUsers[msg.From.ID] {
		b.logger.Printf("Dropped message from unauthorized user %d (%s)", msg.From.ID, msg.From.UserName)
		return
	}

	if msg.IsCommand() {
		b.handleCommand(msg)
		return
	}

	// Non-command text: treat as a search query
	if msg.Text != "" {
		reply, err := cmdSearch(msg.Text)
		if err != nil {
			b.sendMarkdown(msg.Chat.ID, fmt.Sprintf("❌ Error: %s", err))
			return
		}
		b.sendMarkdown(msg.Chat.ID, reply)
	}
}

func (b *Bot) handleCallback(cb *tgbotapi.CallbackQuery) {
	if !b.allowedUsers[cb.From.ID] {
		return
	}

	// Acknowledge the callback
	callback := tgbotapi.NewCallback(cb.ID, "")
	b.api.Request(callback)

	b.logger.Printf("Callback: %s from user %d", cb.Data, cb.From.ID)

	// TODO: Handle approve/note/vault callbacks
	response := tgbotapi.NewMessage(cb.Message.Chat.ID, fmt.Sprintf("Action received: %s", cb.Data))
	b.api.Send(response)
}

func (b *Bot) sendMarkdown(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	if _, err := b.api.Send(msg); err != nil {
		// Retry without markdown if parsing fails
		msg.ParseMode = ""
		if _, err2 := b.api.Send(msg); err2 != nil {
			b.logger.Printf("Failed to send message: %v", err2)
		}
	}
}
