package bot

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/audit"
	"github.com/sgx-labs/same-telegram/internal/config"
	"github.com/sgx-labs/same-telegram/internal/filter"
	"github.com/sgx-labs/same-telegram/internal/notify"
	"github.com/sgx-labs/same-telegram/internal/store"
)

// Bot wraps the Telegram bot API with SAME-specific functionality.
type Bot struct {
	api    *tgbotapi.BotAPI
	cfg    *config.Config
	logger *log.Logger

	// allowedUsers is a set of allowed Telegram user IDs for fast lookup.
	allowedUsers map[int64]bool

	ai         *aiState
	claude     *claudeToggle
	sessions   *sessionStore
	filter     *filter.Filter
	replies    *replyTracker
	store      *store.Store
	onboarding *onboardingState
	auditLog   *audit.Logger

	// inflightMu protects the inflight map.
	inflightMu sync.Mutex
	// inflight stores cancel functions for in-flight AI/CLI requests, keyed by user ID.
	inflight map[int64]inflightEntry
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

	// Open user store (encryption key from config, or auto-generated)
	encKey, err := config.LoadOrGenerateEncryptionKey(cfg.Bot.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("resolve encryption key: %w", err)
	}
	userStore, err := store.New(encKey)
	if err != nil {
		return nil, fmt.Errorf("open user store: %w", err)
	}

	logger.Printf("Authorized on Telegram as @%s", api.Self.UserName)

	return &Bot{
		api:          api,
		cfg:          cfg,
		logger:       logger,
		allowedUsers: allowed,
		ai:           newAIState(),
		claude:       newClaudeToggle(),
		sessions:     newSessionStore(),
		filter:       filter.New(),
		replies:      newReplyTracker(),
		store:        userStore,
		onboarding:   newOnboardingState(),
		inflight:     make(map[int64]inflightEntry),
		auditLog:     audit.NewLogger(),
	}, nil
}

// Run starts the bot polling loop. Blocks until context is cancelled.
func (b *Bot) Run(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	// Register commands with Telegram's "/" menu.
	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "claude", Description: "Toggle Claude mode"},
		tgbotapi.BotCommand{Command: "ai", Description: "AI backend settings"},
		tgbotapi.BotCommand{Command: "reset", Description: "Clear conversation session"},
		tgbotapi.BotCommand{Command: "onboard", Description: "Set up AI backend"},
		tgbotapi.BotCommand{Command: "settings", Description: "Manage settings"},
		tgbotapi.BotCommand{Command: "team", Description: "Agent team status"},
		tgbotapi.BotCommand{Command: "decisions", Description: "Pending decisions"},
		tgbotapi.BotCommand{Command: "announce", Description: "Post CEO announcement"},
		tgbotapi.BotCommand{Command: "reviews", Description: "List pending reviews"},
		tgbotapi.BotCommand{Command: "review", Description: "Read a review doc"},
		tgbotapi.BotCommand{Command: "approve", Description: "Approve a review"},
		tgbotapi.BotCommand{Command: "reject", Description: "Reject a review"},
		tgbotapi.BotCommand{Command: "status", Description: "Vault status"},
		tgbotapi.BotCommand{Command: "doctor", Description: "Health check"},
		tgbotapi.BotCommand{Command: "search", Description: "Search vault"},
		tgbotapi.BotCommand{Command: "ask", Description: "Ask SAME a question"},
		tgbotapi.BotCommand{Command: "stop", Description: "Cancel in-flight request"},
		tgbotapi.BotCommand{Command: "help", Description: "Show all commands"},
	)
	if _, err := b.api.Request(commands); err != nil {
		b.logger.Printf("Failed to register bot commands menu: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			if b.store != nil {
				b.store.Close()
			}
			return ctx.Err()

		case update := <-updates:
			if update.Message != nil {
				b.handleUpdate(update.Message)
			}
			if update.EditedMessage != nil {
				b.handleEditedMessage(update.EditedMessage)
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
	text = b.filter.Sanitize(text)

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

	// Check if this is a reply to an agent message
	if b.HandleReply(msg) {
		return
	}

	if msg.IsCommand() {
		b.handleCommand(msg)
		return
	}

	// Non-command text
	if msg.Text != "" {
		// Check if user is in onboarding flow (awaiting API key or model)
		if b.handleOnboardingInput(msg) {
			return
		}

		// Check API-based AI mode (user configured via onboarding with API key)
		if b.handleAPIAIMessage(msg) {
			return
		}

		// Legacy: CLI-based AI mode, then Claude mode
		if b.ai.isEnabled(msg.From.ID) {
			b.handleAIMessage(msg.Chat.ID, msg.From.ID, msg.Text)
			return
		}
		if b.claude.isEnabled(msg.From.ID) {
			b.handleClaudeMessage(msg.Chat.ID, msg.From.ID, msg.Text)
			return
		}

		// Default: vault search
		reply, err := cmdSearch(msg.Text)
		if err != nil {
			b.sendMarkdown(msg.Chat.ID, fmt.Sprintf("Error: %s", err))
			return
		}
		b.sendMarkdown(msg.Chat.ID, reply)
	}
}

// validCallbackPattern matches expected callback data formats to prevent injection.
var validCallbackPattern = regexp.MustCompile(`^[a-zA-Z0-9_:.\-]{1,64}$`)

func (b *Bot) handleCallback(cb *tgbotapi.CallbackQuery) {
	if !b.allowedUsers[cb.From.ID] {
		return
	}

	// Acknowledge the callback
	callback := tgbotapi.NewCallback(cb.ID, "")
	b.api.Request(callback)

	// Validate callback data format before processing
	if !validCallbackPattern.MatchString(cb.Data) {
		b.logger.Printf("Rejected invalid callback data from user %d: %q", cb.From.ID, cb.Data)
		return
	}

	b.logger.Printf("Callback: %s from user %d", cb.Data, cb.From.ID)

	// Route callbacks by prefix
	switch {
	case strings.HasPrefix(cb.Data, "onboard:"):
		b.handleOnboardingCallback(cb)
	case strings.HasPrefix(cb.Data, "mode:"):
		b.handleModeCallback(cb)
	case strings.HasPrefix(cb.Data, "settings:"):
		b.handleSettingsCallback(cb)
	case strings.HasPrefix(cb.Data, "approve:"), strings.HasPrefix(cb.Data, "reject:"):
		parts := strings.SplitN(cb.Data, ":", 2)
		if len(parts) == 2 {
			action := "approved"
			if parts[0] == "reject" {
				action = "rejected"
			}
			b.handleDecisionAction(cb.Message.Chat.ID, parts[1], action)
		}
	default:
		b.logger.Printf("Unknown callback data %q from user %d — ignoring", cb.Data, cb.From.ID)
	}
}

func (b *Bot) sendMarkdown(chatID int64, text string) {
	if text == "" {
		b.logger.Printf("sendMarkdown called with empty text for chat %d — skipping", chatID)
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	if _, err := b.api.Send(msg); err != nil {
		b.logger.Printf("Markdown send failed (chat %d): %v — retrying as plain text", chatID, err)
		// Retry without markdown if parsing fails
		msg.ParseMode = ""
		if _, err2 := b.api.Send(msg); err2 != nil {
			b.logger.Printf("Plain text send also failed (chat %d): %v", chatID, err2)
		}
	}
}

// deleteMessage deletes a message from the chat (used for API key security).
func (b *Bot) deleteMessage(chatID int64, messageID int) {
	del := tgbotapi.NewDeleteMessage(chatID, messageID)
	if _, err := b.api.Request(del); err != nil {
		b.logger.Printf("Failed to delete message %d: %v", messageID, err)
	}
}
