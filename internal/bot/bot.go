package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/analytics"
	"github.com/sgx-labs/same-telegram/internal/audit"
	"github.com/sgx-labs/same-telegram/internal/config"
	"github.com/sgx-labs/same-telegram/internal/filter"
	"github.com/sgx-labs/same-telegram/internal/machines"
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

	ai            *aiState
	claude        *claudeToggle
	sessions      *sessionStore
	conversations *conversationStore
	filter        *filter.Filter
	replies       *replyTracker
	store         *store.Store
	onboarding    *onboardingState
	auditLog      *audit.Logger
	limiter       *rateLimiter
	inbound       *inboundLimiter

	// orchestrator manages per-user workspace machines (nil if not in workspace mode).
	orchestrator *machines.Orchestrator
	// machineStore is the backing store for the orchestrator (nil if not in workspace mode).
	machineStore *machines.SQLiteStore

	// analytics tracks anonymous usage events (nil if init fails — non-fatal).
	analytics *analytics.Store

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

	limiter := newRateLimiter()
	// Inbound: 1 msg/sec, burst 3 (internal); 1 msg/3sec, burst 2 (public)
	inbound := newInboundLimiter(1.0, 3)
	if cfg.Bot.IsPublicMode() {
		limiter = newPublicRateLimiter()
		inbound = newInboundLimiter(1.0/3.0, 2)
		logger.Printf("Running in PUBLIC mode with stricter rate limits")
	}

	b := &Bot{
		api:          api,
		cfg:          cfg,
		logger:       logger,
		allowedUsers: allowed,
		ai:           newAIState(),
		claude:       newClaudeToggle(),
		sessions:      newSessionStore(),
		conversations: newConversationStore(),
		filter:       filter.New(),
		replies:      newReplyTracker(),
		store:        userStore,
		onboarding:   newOnboardingState(),
		inflight:     make(map[int64]inflightEntry),
		auditLog:     audit.NewLogger(),
		limiter:      limiter,
		inbound:      inbound,
	}

	// Initialize the machine orchestrator in workspace mode.
	if cfg.Bot.IsWorkspaceMode() {
		if err := b.initOrchestrator(); err != nil {
			return nil, fmt.Errorf("init orchestrator: %w", err)
		}
	}

	// Initialize analytics store (non-fatal — bot works without it).
	analyticsPath := expandHome("~/.same/analytics.db")
	if a, err := analytics.New(analyticsPath); err != nil {
		logger.Printf("Analytics disabled: %v", err)
	} else {
		b.analytics = a
		logger.Printf("Analytics enabled: %s", analyticsPath)
	}

	return b, nil
}

// isPublicMode returns true when the bot is running in public (restricted) mode.
func (b *Bot) isPublicMode() bool {
	return b.cfg.Bot.IsPublicMode()
}

// isWorkspaceMode returns true when the bot is running in workspace mode.
func (b *Bot) isWorkspaceMode() bool {
	return b.cfg.Bot.IsWorkspaceMode()
}

// initOrchestrator creates the MachineStore and Fly Machines client for workspace mode.
func (b *Bot) initOrchestrator() error {
	dbPath := expandHome(b.cfg.Bot.MachineDBPath)

	ms, err := machines.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("open machine store: %w", err)
	}
	b.machineStore = ms

	client := machines.NewClient(
		b.cfg.Bot.FlyAppName,
		b.cfg.Bot.FlyAPIToken,
		b.cfg.Bot.FlyImage,
		b.cfg.Bot.FlyRegion,
	)
	b.orchestrator = machines.NewOrchestrator(client, ms)

	b.logger.Printf("Workspace mode: orchestrator ready (app=%s region=%s db=%s)",
		b.cfg.Bot.FlyAppName, b.cfg.Bot.FlyRegion, dbPath)
	return nil
}

// expandHome replaces a leading "~/" with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// Run starts the bot polling loop. Blocks until context is cancelled.
func (b *Bot) Run(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	// Start background eviction for in-memory stores to prevent OOM.
	b.conversations.StartEviction(ctx)
	b.sessions.StartEviction(ctx)
	b.onboarding.StartEviction(ctx)

	// Register commands with Telegram's "/" menu, filtered by mode.
	cmds := commandsForMode(b.cfg.Bot.Mode)
	setCmd := tgbotapi.NewSetMyCommands(cmds...)
	if _, err := b.api.Request(setCmd); err != nil {
		b.logger.Printf("Failed to register bot commands menu: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			if b.store != nil {
				b.store.Close()
			}
			if b.machineStore != nil {
				b.machineStore.Close()
			}
			if b.analytics != nil {
				b.analytics.Close()
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
		if _, err := b.sendWithRetry(msg); err != nil {
			b.logger.Printf("Markdown notification send failed (%d): %v — retrying as plain text", userID, err)
			msg.ParseMode = ""
			if _, err2 := b.sendWithRetry(msg); err2 != nil {
				b.logger.Printf("Plain text notification also failed (%d): %v", userID, err2)
			}
		}
	}
}

func (b *Bot) handleUpdate(msg *tgbotapi.Message) {
	// Security: in internal mode, silently drop messages from unknown users.
	// In public and workspace modes, allow all users (open access).
	if !b.isPublicMode() && !b.isWorkspaceMode() && !b.allowedUsers[msg.From.ID] {
		b.logger.Printf("Dropped message from unauthorized user %d (%s)", msg.From.ID, msg.From.UserName)
		return
	}

	// Inbound rate limiting: silently drop messages that exceed the per-user rate.
	// No response is sent to avoid amplification attacks.
	if !b.inbound.allow(msg.From.ID) {
		b.logger.Printf("Inbound rate limit exceeded for user %d — dropping message", msg.From.ID)
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
		// Check if user is confirming workspace destruction.
		if b.isWorkspaceMode() && b.handleDestroyConfirmation(msg) {
			return
		}

		// Check if user pasted an invite code.
		if b.isWorkspaceMode() && b.handleInviteCodeInput(msg) {
			return
		}

		// Check if user is in workspace onboarding topic step
		if b.isWorkspaceMode() && b.handleWorkspaceTopicInput(msg) {
			return
		}

		// Check if user is in onboarding flow (awaiting API key or model)
		if b.handleOnboardingInput(msg) {
			return
		}

		// Check API-based AI mode (user configured via onboarding with API key)
		if b.handleAPIAIMessage(msg) {
			return
		}

		// Legacy: CLI-based AI mode, then Claude mode (internal only — shells out)
		if !b.isPublicMode() {
			if b.ai.isEnabled(msg.From.ID) {
				b.handleAIMessage(msg.Chat.ID, msg.From.ID, msg.Text)
				return
			}
			if b.claude.isEnabled(msg.From.ID) {
				b.handleClaudeMessage(msg.Chat.ID, msg.From.ID, msg.Text)
				return
			}
		}

		// Default behavior depends on mode.
		if b.isWorkspaceMode() {
			if b.requiresInviteCode() && !b.hasValidInvite(msg.From.ID) {
				b.sendMarkdown(msg.Chat.ID, "If you have an invite code, paste it here.")
			} else {
				b.sendMarkdown(msg.Chat.ID, "Use /start to set up your workspace.")
			}
		} else if !b.isPublicMode() {
			// Internal mode: vault search (shells out to `same search`)
			reply, err := cmdSearch(msg.Text)
			if err != nil {
				b.sendMarkdown(msg.Chat.ID, fmt.Sprintf("Error: %s", err))
				return
			}
			b.sendMarkdown(msg.Chat.ID, reply)
		} else {
			// Public mode: auto-trigger onboarding for users who haven't set up yet
			b.sendOnboardingPrompt(msg.Chat.ID)
		}
	}
}

// validCallbackPattern matches expected callback data formats to prevent injection.
var validCallbackPattern = regexp.MustCompile(`^[a-zA-Z0-9_:.\-]{1,64}$`)

func (b *Bot) handleCallback(cb *tgbotapi.CallbackQuery) {
	if !b.isPublicMode() && !b.isWorkspaceMode() && !b.allowedUsers[cb.From.ID] {
		return
	}

	// Inbound rate limiting for callbacks — silently drop if exceeded.
	if !b.inbound.allow(cb.From.ID) {
		b.logger.Printf("Inbound rate limit exceeded for user %d callback — dropping", cb.From.ID)
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
	case strings.HasPrefix(cb.Data, "ws:"):
		b.handleWorkspaceCallback(cb)
	case strings.HasPrefix(cb.Data, "onboard:"):
		b.handleOnboardingCallback(cb)
	case strings.HasPrefix(cb.Data, "mode:"):
		if b.isPublicMode() && strings.Contains(cb.Data, ":cli:") {
			b.sendMarkdown(cb.Message.Chat.ID, "CLI mode is not available. Use API Key mode instead.")
			return
		}
		b.handleModeCallback(cb)
	case strings.HasPrefix(cb.Data, "settings:"):
		b.handleSettingsCallback(cb)
	case strings.HasPrefix(cb.Data, "review_approve:"), strings.HasPrefix(cb.Data, "review_reject:"):
		parts := strings.SplitN(cb.Data, ":", 2)
		if len(parts) == 2 {
			if b.isPublicMode() {
				b.sendMarkdown(cb.Message.Chat.ID, "This command is not available.")
				return
			}
			action := "approved"
			if strings.HasPrefix(cb.Data, "review_reject:") {
				action = "rejected"
			}
			result, err := moveReview(parts[1], action)
			if err != nil {
				b.sendMarkdown(cb.Message.Chat.ID, fmt.Sprintf("Error: %s", err))
			} else {
				b.sendMarkdown(cb.Message.Chat.ID, result)
			}
		}
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
	if _, err := b.sendWithRetry(msg); err != nil {
		b.logger.Printf("Markdown send failed (chat %d): %v — retrying as plain text", chatID, err)
		// Retry without markdown if parsing fails
		msg.ParseMode = ""
		if _, err2 := b.sendWithRetry(msg); err2 != nil {
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
