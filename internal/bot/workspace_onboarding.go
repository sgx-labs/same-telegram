package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/analytics"
)

// Workspace onboarding flow:
//
//   1. User taps /start
//   2. Bot kicks off machine provisioning in the background
//   3. Bot asks: "What kind of vault?" [Dev Tools] [IT Ops] [API Design] [Empty]
//   4. User picks a seed vault
//   5. Bot asks: "How do you want to connect your AI?"
//      [API Key] [Prepaid credits]
//   6. User completes auth
//   7. Bot sends: "Your workspace is ready" + [Open Terminal] button
//
// By step 7, the machine has been provisioning for 30-60 seconds (the time it
// takes the user to answer the questions). It should be ready or nearly ready.

// workspaceOnboardingState tracks per-user workspace onboarding progress.
type workspaceOnboardingState struct {
	step      string // "seed", "topic", "auth", "provider", "key", "invite", "ready"
	seed      string // selected seed vault
	topic     string // user-provided topic/project description (for research/project seeds)
	authType  string // "byok", "paygo"
	provider  string // "anthropic", "openrouter" — selected AI provider (BYOK sub-step)
	machineID string // Fly Machine ID (set once provisioning completes)
	flyMachID string // Fly Machine ID (for fly-replay routing)
	token     string // workspace auth token
}

// handleWorkspaceStart handles /start in workspace mode.
// It begins provisioning immediately and starts the onboarding conversation.
func (b *Bot) handleWorkspaceStart(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	userID := msg.From.ID
	userIDStr := strconv.FormatInt(userID, 10)

	// Access control: if allowlist is configured, isBlockedUser already rejected
	// unauthorized users in handleUpdate. If invite codes are also configured
	// (no allowlist), fall back to the invite code flow.
	payload := msg.CommandArguments()
	if !b.hasAllowlist() && b.requiresInviteCode() && !b.hasValidInvite(userID) {
		if strings.HasPrefix(payload, "invite_") {
			code := strings.TrimPrefix(payload, "invite_")
			if b.validateInviteCode(code) {
				b.markInviteUsed(userID, code)
			} else {
				b.sendMarkdown(chatID, "That invite code isn't valid. Ask your friend for a new one.")
				return
			}
		} else {
			b.sendMarkdown(chatID,
				"*SameVault* is invite-only right now.\n\n"+
					"If you have an invite code, paste it here.")
			return
		}
	}

	// Check if user already has a workspace.
	if b.orchestrator != nil {
		existing, err := b.orchestrator.Status(context.Background(), userIDStr)
		if err == nil && existing != nil {
			// Already set up — just send the terminal button.
			b.sendWorkspaceReady(chatID, existing.Token, existing.MachineID)
			return
		}
	}

	// --- Start provisioning in the background ---
	// This runs while the user answers onboarding questions.
	if b.orchestrator != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			machID, token, err := b.orchestrator.EnsureRunning(ctx, userIDStr)
			if err != nil {
				b.logger.Printf("background provisioning failed for user %d: %v", userID, err)
				b.sendMarkdown(chatID, fmt.Sprintf("Something went wrong setting up your workspace: %v\n\nTry /start again.", err))
				return
			}

			// Store the result for when onboarding completes.
			b.onboarding.mu.Lock()
			if ws, ok := b.onboarding.workspaces[userID]; ok {
				ws.machineID = "provisioned"
				ws.flyMachID = machID
				ws.token = token
			} else {
				b.onboarding.workspaces[userID] = &workspaceOnboardingState{
					machineID: "provisioned",
					flyMachID: machID,
					token:     token,
				}
			}
			b.onboarding.mu.Unlock()

			b.logger.Printf("workspace provisioned for user %d: machine=%s", userID, machID)
		}()
	} else {
		b.logger.Printf("orchestrator is nil for user %d — workspace provisioning skipped (bot misconfigured?)", userID)
		b.sendMarkdown(chatID, "Workspace provisioning is not available right now. Please contact support.")
	}

	// --- Send seed vault selection ---
	b.onboarding.mu.Lock()
	if b.onboarding.workspaces == nil {
		b.onboarding.workspaces = make(map[int64]*workspaceOnboardingState)
	}
	b.onboarding.workspaces[userID] = &workspaceOnboardingState{step: "seed"}
	b.onboarding.timestamps[userID] = time.Now()
	b.onboarding.mu.Unlock()

	welcome := "*Welcome to SameVault*\n\n" +
		"Your cloud workspace is a persistent AI environment that remembers " +
		"everything across sessions.\n\n" +
		"I'll walk you through setup. First, pick a starting point — " +
		"you can always add more vault packs later.\n\n" +
		"_We collect anonymous usage stats to improve the product. " +
		"Tap /privacy anytime to opt out._\n\n" +
		"*Recommended:*"

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("SAME Demo — guided setup tour", "ws:seed:same-demo"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Research — establish research topics", "ws:seed:research"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Project — set up a coding project", "ws:seed:project"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Bot Dev — build Telegram bots", "ws:seed:bot-dev"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Empty — start from scratch", "ws:seed:empty"),
		),
	)

	m := tgbotapi.NewMessage(chatID, welcome)
	m.ParseMode = "Markdown"
	m.ReplyMarkup = kb
	b.api.Send(m)
}

// handleWorkspaceCallback processes workspace onboarding button presses.
func (b *Bot) handleWorkspaceCallback(cb *tgbotapi.CallbackQuery) {
	chatID := cb.Message.Chat.ID
	userID := cb.From.ID
	data := strings.TrimPrefix(cb.Data, "ws:")

	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return
	}
	action := parts[0]
	value := parts[1]

	// Check if the user still has an active onboarding session.
	b.onboarding.mu.Lock()
	ws := b.onboarding.workspaces[userID]
	b.onboarding.mu.Unlock()
	if ws == nil {
		b.logger.Printf("workspace callback from user %d with no active session (action=%s)", userID, action)
		b.sendMarkdown(chatID, "Your session expired. Tap /start to begin again.")
		return
	}

	switch action {
	case "seed":
		b.handleWorkspaceSeedSelection(chatID, userID, value)
	case "auth":
		b.handleWorkspaceAuthSelection(chatID, userID, value)
	case "provider":
		b.handleWorkspaceProviderSelection(chatID, userID, value)
	}
}

// handleWorkspaceSeedSelection processes seed vault selection.
func (b *Bot) handleWorkspaceSeedSelection(chatID, userID int64, seed string) {
	b.logEvent(userID, analytics.EventSeedSelected, seed)

	// Store selection.
	b.onboarding.mu.Lock()
	ws, ok := b.onboarding.workspaces[userID]
	if !ok || ws == nil {
		b.onboarding.mu.Unlock()
		b.logger.Printf("workspace state missing for user %d during seed selection", userID)
		b.sendMarkdown(chatID, "Your session has expired. Tap /start to begin again.")
		return
	}
	ws.seed = seed
	b.onboarding.mu.Unlock()

	// Seeds that benefit from a topic description get an extra input step.
	switch seed {
	case "research":
		b.onboarding.mu.Lock()
		if ws, ok := b.onboarding.workspaces[userID]; ok {
			ws.step = "topic"
		}
		b.onboarding.mu.Unlock()
		b.sendMarkdown(chatID,
			"What would you like to research? Describe your topic and I'll set up your vault with key concepts, open questions, and starting points.")
		return
	case "project":
		b.onboarding.mu.Lock()
		if ws, ok := b.onboarding.workspaces[userID]; ok {
			ws.step = "topic"
		}
		b.onboarding.mu.Unlock()
		b.sendMarkdown(chatID,
			"What are you building? Describe your project and I'll scaffold your workspace with the right structure and context.")
		return
	case "bot-dev":
		b.onboarding.mu.Lock()
		if ws, ok := b.onboarding.workspaces[userID]; ok {
			ws.step = "topic"
		}
		b.onboarding.mu.Unlock()
		b.sendMarkdown(chatID,
			"What kind of bot are you building? Describe your bot idea and I'll set up your workspace with the right patterns and references.")
		return
	default:
		// same-demo, empty, and any future seeds skip the topic step.
		b.onboarding.mu.Lock()
		if ws, ok := b.onboarding.workspaces[userID]; ok {
			ws.step = "auth"
		}
		b.onboarding.mu.Unlock()
		b.sendWorkspaceAuthPrompt(chatID, seed)
	}
}

// handleWorkspaceTopicInput captures the user's free-text topic description
// and advances to the auth selection step. Returns true if the message was handled.
func (b *Bot) handleWorkspaceTopicInput(msg *tgbotapi.Message) bool {
	userID := msg.From.ID
	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)

	b.onboarding.mu.Lock()
	ws := b.onboarding.workspaces[userID]
	if ws == nil {
		// Check if the user had a workspace state at some point (timestamp exists).
		// If so, it expired. Otherwise, this message isn't for us.
		_, hadState := b.onboarding.timestamps[userID]
		b.onboarding.mu.Unlock()
		if hadState {
			b.logger.Printf("workspace state expired for user %d during topic input", userID)
			b.sendMarkdown(chatID, "Your session has expired. Tap /start to begin again.")
			return true
		}
		return false
	}
	if ws.step != "topic" {
		b.onboarding.mu.Unlock()
		return false
	}
	ws.topic = text
	ws.step = "auth"
	seed := ws.seed
	b.onboarding.mu.Unlock()

	b.logEvent(userID, analytics.EventTopicProvided, seed)

	// Show a brief confirmation with a truncated preview.
	preview := text
	if len(preview) > 50 {
		preview = preview[:50] + "..."
	}
	b.sendMarkdown(chatID, fmt.Sprintf("Got it — *%s* I'll set this up in your workspace.", preview))

	b.sendWorkspaceAuthPrompt(chatID, seed)
	return true
}

// sendWorkspaceAuthPrompt sends the auth method selection keyboard.
func (b *Bot) sendWorkspaceAuthPrompt(chatID int64, seed string) {
	seedName := seedDisplayName(seed)
	text := fmt.Sprintf("*%s* — got it.\n\n"+
		"You'll sign into Claude Code in the terminal.\n"+
		"Just run `claude login` when it opens.",
		seedName)

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Open my workspace", "ws:auth:skip"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("I have an API key instead", "ws:auth:byok"),
		),
	)

	m := tgbotapi.NewMessage(chatID, text)
	m.ParseMode = "Markdown"
	m.ReplyMarkup = kb
	b.api.Send(m)
}

// handleWorkspaceAuthSelection processes auth method selection.
func (b *Bot) handleWorkspaceAuthSelection(chatID, userID int64, authType string) {
	b.logEvent(userID, analytics.EventAuthSelected, authType)

	b.onboarding.mu.Lock()
	ws := b.onboarding.workspaces[userID]
	if ws != nil {
		ws.authType = authType
	} else {
		b.onboarding.mu.Unlock()
		b.logger.Printf("workspace state missing for user %d during auth selection", userID)
		b.sendMarkdown(chatID, "Your session has expired. Tap /start to begin again.")
		return
	}
	b.onboarding.mu.Unlock()

	switch authType {
	case "byok":
		// Show provider selection: Anthropic or OpenRouter.
		b.sendProviderSelection(chatID, userID)

	case "skip":
		// Skip AI setup for now — go straight to workspace.
		b.handleWorkspaceComplete(chatID, userID)
	}
}

// sendProviderSelection shows the AI provider choice (Anthropic vs OpenRouter).
func (b *Bot) sendProviderSelection(chatID, userID int64) {
	b.onboarding.mu.Lock()
	if ws, ok := b.onboarding.workspaces[userID]; ok {
		ws.step = "provider"
	}
	b.onboarding.mu.Unlock()

	text := "*Choose your AI provider*\n\n" +
		"*Anthropic (Claude)* — direct API access to Claude models\n" +
		"Get a key at console.anthropic.com\n\n" +
		"*OpenRouter (100+ models)* — one key for Claude, GPT-4, Gemini, Llama, and more\n" +
		"Get a key at openrouter.ai"

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Anthropic (Claude)", "ws:provider:anthropic"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("OpenRouter (100+ models)", "ws:provider:openrouter"),
		),
	)

	m := tgbotapi.NewMessage(chatID, text)
	m.ParseMode = "Markdown"
	m.ReplyMarkup = kb
	b.api.Send(m)
}

// handleWorkspaceProviderSelection processes the AI provider choice callback.
func (b *Bot) handleWorkspaceProviderSelection(chatID, userID int64, provider string) {
	b.onboarding.mu.Lock()
	ws := b.onboarding.workspaces[userID]
	if ws != nil {
		ws.provider = provider
		ws.step = "key"
	}
	b.onboarding.mu.Unlock()

	switch provider {
	case "openrouter":
		b.onboarding.setAwaitingKey(userID, "openrouter")
		b.sendMarkdown(chatID,
			"*OpenRouter selected*\n\n"+
				"One API key for Claude, GPT-4, Gemini, Llama, and 100+ more models.\n\n"+
				"*How to get a key:*\n"+
				"1. Sign up at https://openrouter.ai/?ref=samevault\n"+
				"2. Go to *Keys* in the dashboard\n"+
				"3. Click *Create Key*\n"+
				"4. Copy the key and paste it here\n\n"+
				"Expected format: `sk-or-...`\n\n"+
				"Your key will be encrypted and injected into your workspace.\n"+
				"Your message will be deleted for safety.\n\n"+
				"Send /cancel to go back.")

	default: // "anthropic"
		b.onboarding.setAwaitingKey(userID, "claude")
		b.sendMarkdown(chatID,
			"*Paste your API key*\n\n"+
				"Send your Anthropic or OpenAI key. "+
				"I'll detect the provider automatically.\n\n"+
				"It will be encrypted and injected into your workspace so "+
				"Claude Code (or other tools) work immediately.\n\n"+
				"Your message will be deleted for safety.\n\n"+
				"Send /cancel to go back.")
	}
}

// handleWorkspaceComplete finishes onboarding and presents the terminal button.
func (b *Bot) handleWorkspaceComplete(chatID, userID int64) {
	b.onboarding.mu.Lock()
	ws := b.onboarding.workspaces[userID]
	b.onboarding.mu.Unlock()

	if ws == nil {
		b.sendMarkdown(chatID, "Something went wrong. Try /start again.")
		return
	}

	userIDStr := strconv.FormatInt(userID, 10)

	// Check if provisioning completed.
	if ws.token != "" {
		b.sendWorkspaceReady(chatID, ws.token, ws.flyMachID)
		b.triggerVaultSeed(chatID, userID, userIDStr, ws.seed, ws.topic)
		// Track bot/workspace creation complete (non-blocking).
		b.trackEvent(userID, analytics.EventBotCreated, map[string]string{
			"user_id":  strconv.FormatInt(userID, 10),
			"template": ws.seed,
		})
		// If user skipped auth, tell them how to set up AI in the terminal.
		if ws.authType == "skip" {
			b.sendMarkdown(chatID,
				"*To use AI in your workspace:*\n\n"+
					"`claude login` — sign in with your Claude account\n"+
					"`export ANTHROPIC_API_KEY=sk-ant-...` — or use an Anthropic key\n"+
					"`export OPENROUTER_API_KEY=sk-or-...` — or use OpenRouter for 100+ models\n\n"+
					"Get an OpenRouter key: https://openrouter.ai/?ref=samevault\n\n"+
					"Your login persists across sessions.")
		}
		return
	}

	// Provisioning still running — show a waiting message.
	b.sendMarkdown(chatID,
		"Your workspace is almost ready — just a few more seconds...\n\n"+
			"_Setting up your vault and connecting your AI tools._")

	// Capture state before entering the goroutine so values
	// remain stable even if the onboarding state is mutated later.
	seed := ws.seed
	topic := ws.topic
	authType := ws.authType

	// Poll for completion (up to 30 seconds).
	go func() {
		sentIntermediate := false
		for i := 0; i < 15; i++ {
			time.Sleep(2 * time.Second)

			b.onboarding.mu.Lock()
			ws := b.onboarding.workspaces[userID]
			token := ""
			machID := ""
			if ws != nil {
				token = ws.token
				machID = ws.flyMachID
			}
			b.onboarding.mu.Unlock()

			if token != "" {
				b.sendWorkspaceReady(chatID, token, machID)
				b.triggerVaultSeed(chatID, userID, userIDStr, seed, topic)
				if authType == "skip" {
					b.sendMarkdown(chatID,
						"*To use AI in your workspace:*\n\n"+
							"`claude login` — sign in with your Claude account\n"+
							"`export ANTHROPIC_API_KEY=sk-ant-...` — or use an Anthropic key\n"+
							"`export OPENROUTER_API_KEY=sk-or-...` — or use OpenRouter for 100+ models\n\n"+
							"Get an OpenRouter key: https://openrouter.ai/?ref=samevault")
				}
				return
			}

			// After ~10 seconds (5 iterations x 2s), send an intermediate update.
			if i == 4 && !sentIntermediate {
				sentIntermediate = true
				b.sendMarkdown(chatID, "Still working on your workspace...")
			}
		}

		// Timed out — let them know.
		b.sendMarkdown(chatID,
			"Your workspace is taking longer than expected to start. "+
				"Try /start again in a minute, or contact support if this keeps happening.")
	}()
}

// triggerVaultSeed runs the vault seed script inside the user's workspace container
// in the background. It is a no-op if the orchestrator is nil or the seed is empty.
func (b *Bot) triggerVaultSeed(chatID, userID int64, userIDStr, seed, topic string) {
	if b.orchestrator == nil {
		b.logger.Printf("triggerVaultSeed: orchestrator is nil for user %d, skipping", userID)
		return
	}
	go func() {
		if err := b.orchestrator.SeedVault(context.Background(), userIDStr, seed, topic); err != nil {
			b.logger.Printf("vault seeding failed for user %d: %v", userID, err)
			b.sendMarkdown(chatID,
				"Your workspace is ready but we couldn't set up the seed vault. "+
					"You can start from scratch.")
		}
	}()
}

// webAppInfo mirrors Telegram's WebAppInfo type for Mini App buttons.
// The upstream go-telegram-bot-api v5.5.1 predates WebApp support,
// so we define the minimal types needed for JSON serialization.
type webAppInfo struct {
	URL string `json:"url"`
}

// webAppInlineButton is an InlineKeyboardButton with WebApp support.
type webAppInlineButton struct {
	Text   string      `json:"text"`
	WebApp *webAppInfo `json:"web_app,omitempty"`
}

// webAppInlineKeyboardMarkup wraps rows of webAppInlineButton.
type webAppInlineKeyboardMarkup struct {
	InlineKeyboard [][]webAppInlineButton `json:"inline_keyboard"`
}

// sendWorkspaceReady sends the "workspace ready" message with the terminal button.
// machineID is included in the URL so Fly's proxy can route to the correct machine.
func (b *Bot) sendWorkspaceReady(chatID int64, token, machineID string) {
	// Track workspace creation with structured properties.
	b.trackEvent(chatID, analytics.EventWorkspaceCreated, map[string]string{
		"user_id":    strconv.FormatInt(chatID, 10),
		"region":     b.cfg.Bot.FlyRegion,
		"machine_id": machineID,
	})
	terminalURL := fmt.Sprintf("https://%s/?token=%s&instance=%s", b.workspaceHost(), token, machineID)
	b.logger.Printf("sendWorkspaceReady: chat=%d machine=%s host=%s", chatID, machineID, b.workspaceHost())

	text := "*Your workspace is ready.*\n\n" +
		"Tap below to open your terminal.\n\n" +
		"_Your session persists — close and reopen anytime._"

	kb := webAppInlineKeyboardMarkup{
		InlineKeyboard: [][]webAppInlineButton{
			{
				{
					Text:   "Open Terminal",
					WebApp: &webAppInfo{URL: terminalURL},
				},
			},
		},
	}

	m := tgbotapi.NewMessage(chatID, text)
	m.ParseMode = "Markdown"
	m.ReplyMarkup = kb
	if _, err := b.api.Send(m); err != nil {
		b.logger.Printf("sendWorkspaceReady: WebApp button send failed for chat %d: %v — falling back to plain URL", chatID, err)
		// Fallback: send the URL as plain text so the user has something.
		fallback := fmt.Sprintf("Your workspace is ready.\n\nOpen your terminal here:\n%s", terminalURL)
		b.sendMarkdown(chatID, fallback)
	}
}

// workspaceHost returns the hostname for the workspace Mini App.
func (b *Bot) workspaceHost() string {
	if b.cfg.Bot.WorkspaceHost != "" {
		return b.cfg.Bot.WorkspaceHost
	}
	return "workspace.samevault.com"
}

// requiresInviteCode returns true if the bot is configured to require invite codes.
func (b *Bot) requiresInviteCode() bool {
	return b.cfg.Bot.InviteCode != ""
}

// validateInviteCode checks if an invite code is valid and hasn't exceeded max uses.
func (b *Bot) validateInviteCode(code string) bool {
	if code == "" || code != b.cfg.Bot.InviteCode {
		return false
	}
	if b.cfg.Bot.MaxUsers > 0 && b.onboarding.inviteCount() >= b.cfg.Bot.MaxUsers {
		return false
	}
	if b.cfg.Bot.MaxUsers == 0 && b.onboarding.inviteCount() >= maxInviteUses {
		return false
	}
	return true
}

// hasValidInvite checks if a user has already used a valid invite code.
func (b *Bot) hasValidInvite(userID int64) bool {
	return b.onboarding.isInvited(userID)
}

// markInviteUsed records that a user used an invite code.
func (b *Bot) markInviteUsed(userID int64, code string) {
	b.onboarding.markInvited(userID)
	b.logEvent(userID, analytics.EventInviteUsed, "")
	b.logger.Printf("user %d used invite code %s (total: %d)", userID, code, b.onboarding.inviteCount())
}

// handleInviteCodeInput checks if a plain text message is a valid invite code.
// If so, it marks the user as invited and kicks off /start. Returns true if handled.
func (b *Bot) handleInviteCodeInput(msg *tgbotapi.Message) bool {
	if !b.requiresInviteCode() || b.hasValidInvite(msg.From.ID) {
		return false
	}
	code := strings.TrimSpace(msg.Text)
	if !b.validateInviteCode(code) {
		return false
	}
	b.markInviteUsed(msg.From.ID, code)
	b.handleWorkspaceStart(msg)
	return true
}

// --- Destroy flow ---

// handleDestroyCommand initiates workspace destruction with confirmation.
func (b *Bot) handleDestroyCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	userID := msg.From.ID
	userIDStr := strconv.FormatInt(userID, 10)

	if b.orchestrator == nil {
		b.sendMarkdown(chatID, "Workspace mode is not enabled.")
		return
	}

	um, err := b.orchestrator.Status(context.Background(), userIDStr)
	if err != nil || um == nil {
		b.sendMarkdown(chatID, "You don't have a workspace to destroy. Use /start to create one.")
		return
	}

	// Set pending confirmation (expires after 60 seconds).
	b.onboarding.mu.Lock()
	b.onboarding.pendingDestroy[userID] = time.Now()
	b.onboarding.mu.Unlock()

	b.sendMarkdown(chatID,
		"*This will permanently destroy your workspace.*\n\n"+
			"All files, vault data, and settings will be deleted. This cannot be undone.\n\n"+
			"Type `DESTROY` to confirm, or anything else to cancel.")
}

// handleDestroyConfirmation checks if a text message is a destroy confirmation.
// Returns true if handled.
func (b *Bot) handleDestroyConfirmation(msg *tgbotapi.Message) bool {
	userID := msg.From.ID

	b.onboarding.mu.Lock()
	requested, pending := b.onboarding.pendingDestroy[userID]
	if pending {
		delete(b.onboarding.pendingDestroy, userID)
	}
	b.onboarding.mu.Unlock()

	if !pending {
		return false
	}

	// Check expiry (60 seconds).
	if time.Since(requested) > 60*time.Second {
		b.sendMarkdown(msg.Chat.ID, "Destroy request expired. Run /destroy again if you still want to proceed.")
		return true
	}

	text := strings.TrimSpace(msg.Text)
	if text != "DESTROY" {
		b.sendMarkdown(msg.Chat.ID, "Destroy cancelled.")
		return true
	}

	// Confirmed — destroy the workspace.
	chatID := msg.Chat.ID
	userIDStr := strconv.FormatInt(userID, 10)

	b.sendMarkdown(chatID, "Destroying your workspace...")

	if err := b.orchestrator.Destroy(context.Background(), userIDStr); err != nil {
		b.logger.Printf("workspace destroy failed for user %d: %v", userID, err)
		b.sendMarkdown(chatID, fmt.Sprintf("Failed to destroy workspace: %v\n\nTry again or contact support.", err))
		return true
	}

	b.sendMarkdown(chatID, "Your workspace has been destroyed.\n\nUse /start to create a new one.")
	b.logger.Printf("user %d destroyed their workspace", userID)
	return true
}

// --- API key setup ---

// injectWorkspaceAPIKey pushes an API key into the user's running workspace
// container so CLI tools (Claude Code, etc.) can use it immediately.
// For OpenRouter, it also injects the base URL env var.
func (b *Bot) injectWorkspaceAPIKey(userID int64, rawKey, backend string) {
	if b.orchestrator == nil {
		return
	}
	envName := envNameForBackend(backend)
	userIDStr := strconv.FormatInt(userID, 10)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := b.orchestrator.InjectAPIKey(ctx, userIDStr, envName, rawKey); err != nil {
			b.logger.Printf("failed to inject API key for user %d: %v", userID, err)
			return
		}
		b.logger.Printf("API key (%s) injected into workspace for user %d", envName, userID)

		// OpenRouter also needs the base URL env var for Claude Code compatibility.
		if backend == "openrouter" {
			if err := b.orchestrator.InjectEnvVar(ctx, userIDStr, "OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"); err != nil {
				b.logger.Printf("failed to inject OPENROUTER_BASE_URL for user %d: %v", userID, err)
			} else {
				b.logger.Printf("OPENROUTER_BASE_URL injected into workspace for user %d", userID)
			}
		}
	}()
}

// detectKeyBackend infers the AI backend from an API key prefix.
func detectKeyBackend(key string) string {
	switch {
	case strings.HasPrefix(key, "sk-ant-"):
		return "claude"
	case strings.HasPrefix(key, "sk-or-"):
		return "openrouter"
	case strings.HasPrefix(key, "sk-"):
		return "openai"
	case strings.HasPrefix(key, "AIzaSy"):
		return "gemini"
	default:
		return "claude" // default to Anthropic — Claude Code is the primary tool
	}
}

// envNameForBackend returns the environment variable name for a given backend.
func envNameForBackend(backend string) string {
	switch backend {
	case "openai":
		return "OPENAI_API_KEY"
	case "gemini":
		return "GOOGLE_API_KEY"
	case "openrouter":
		return "OPENROUTER_API_KEY"
	default:
		return "ANTHROPIC_API_KEY"
	}
}

// maxInviteUses is the maximum number of times the invite code can be used.
const maxInviteUses = 10

// seedDisplayName returns a friendly name for a seed vault.
func seedDisplayName(seed string) string {
	switch seed {
	case "same-demo":
		return "SAME Demo"
	case "research":
		return "Research Workspace"
	case "project":
		return "Project Workspace"
	case "bot-dev":
		return "Bot Developer Workspace"
	case "empty":
		return "Empty Workspace"
	default:
		return seed
	}
}
