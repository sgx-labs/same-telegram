package bot

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/ai"
	"github.com/sgx-labs/same-telegram/internal/store"
)

// BackendOllama is the backend constant for Ollama (no API key needed).
const BackendOllama = "ollama"



// sendOnboardingPrompt sends the welcome message with backend selection buttons.
func (b *Bot) sendOnboardingPrompt(chatID int64) {
	text := "*Welcome to SAME AI Chat*\n\nChoose your AI backend to get started."
	kb := OnboardingKeyboard()
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = kb
	b.api.Send(msg)
}

// OnboardingKeyboard creates the backend selection inline keyboard.
func OnboardingKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Claude", "onboard:claude"),
			tgbotapi.NewInlineKeyboardButtonData("OpenAI", "onboard:openai"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Gemini", "onboard:gemini"),
			tgbotapi.NewInlineKeyboardButtonData("Ollama", "onboard:ollama"),
		),
	)
}

// ModeKeyboard creates the connection mode selection inline keyboard.
func ModeKeyboard(backend string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("API Key", "mode:api:"+backend),
			tgbotapi.NewInlineKeyboardButtonData("CLI (local)", "mode:cli:"+backend),
		),
	)
}

// settingsModeKeyboard returns inline keyboard for switching mode from /settings.
func settingsModeKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Switch to API Key", "settings:mode:api"),
			tgbotapi.NewInlineKeyboardButtonData("Switch to CLI", "settings:mode:cli"),
		),
	)
}

// settingsBackendKeyboard returns inline keyboard for switching backend from /settings.
func settingsBackendKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Claude", "settings:backend:claude"),
			tgbotapi.NewInlineKeyboardButtonData("OpenAI", "settings:backend:openai"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Gemini", "settings:backend:gemini"),
			tgbotapi.NewInlineKeyboardButtonData("Ollama", "settings:backend:ollama"),
		),
	)
}

// handleOnboardingCallback processes backend selection from inline keyboard.
// After backend selection, presents mode selection (API Key vs CLI).
func (b *Bot) handleOnboardingCallback(cb *tgbotapi.CallbackQuery) {
	backend := strings.TrimPrefix(cb.Data, "onboard:")
	chatID := cb.Message.Chat.ID

	// Present mode selection
	text := fmt.Sprintf("*%s selected.*\n\nHow do you want to connect?", backendDisplayName(backend))
	kb := ModeKeyboard(backend)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = kb
	b.api.Send(msg)
}

// handleModeCallback processes the connection mode selection (API Key or CLI).
func (b *Bot) handleModeCallback(cb *tgbotapi.CallbackQuery) {
	// Format: "mode:api:claude" or "mode:cli:claude"
	parts := strings.SplitN(cb.Data, ":", 3)
	if len(parts) < 3 {
		return
	}
	mode := parts[1]   // "api" or "cli"
	backend := parts[2] // "claude", "openai", etc.
	userID := cb.From.ID
	chatID := cb.Message.Chat.ID

	switch mode {
	case "cli":
		b.handleCLIModeSetup(chatID, userID, backend)

	case "api":
		if backend == BackendOllama {
			// Ollama doesn't use API keys -- save directly with API mode
			u := &store.User{
				TelegramUserID: userID,
				Backend:        "ollama",
				Model:          ai.DefaultModel(ai.BackendOllama),
				Mode:           store.ModeAPI,
				AIEnabled:      true,
			}
			if err := b.store.SaveUser(u); err != nil {
				b.sendMarkdown(chatID, fmt.Sprintf("Failed to save settings: %s", err))
				return
			}
			b.sendMarkdown(chatID, fmt.Sprintf(
				"*Ollama configured.*\n\nModel: `%s`\nMode: API\nJust type your message to chat.\n\nMake sure Ollama is running locally.",
				ai.DefaultModel(ai.BackendOllama)))
			return
		}

		// Ask for API key
		b.onboarding.setAwaitingKey(userID, backend)

		keyHint := map[string]string{
			"claude": "sk-ant-api...",
			"openai": "sk-proj-...",
			"gemini": "AIzaSy...",
		}
		b.sendMarkdown(chatID, fmt.Sprintf(
			"*%s selected (API Key mode).*\n\nPlease send your API key now.\n\nExpected format: `%s`\n\n_Your message will be deleted immediately after reading for security._\n\nSend /cancel to abort.",
			backendDisplayName(backend), keyHint[backend]))
	}
}

// handleCLIModeSetup verifies the CLI binary exists on the system and saves the preference.
// CLI mode is restricted to the bot owner because it executes local commands.
func (b *Bot) handleCLIModeSetup(chatID int64, userID int64, backend string) {
	ownerID := b.cfg.Bot.EffectiveOwnerID()
	if ownerID != 0 && userID != ownerID {
		b.sendMarkdown(chatID, "CLI mode is only available to the bot owner. Please use API key mode instead.")
		return
	}

	binary := store.CLIBinaryForBackend(backend)

	path, err := exec.LookPath(binary)
	if err != nil {
		install := map[string]string{
			"claude": "npm install -g @anthropic-ai/claude-code",
			"openai": "npm install -g @openai/codex",
			"gemini": "npm install -g @google/gemini-cli",
			"ollama": "brew install ollama && ollama pull qwen2.5-coder:7b",
		}
		hint := install[backend]
		if hint == "" {
			hint = fmt.Sprintf("Install the %s CLI", binary)
		}
		b.sendMarkdown(chatID, fmt.Sprintf(
			"`%s` CLI not found on this system.\n\nInstall with:\n`%s`\n\nThen try again with /onboard.",
			binary, hint))
		return
	}

	// Save to persistent store
	if b.store != nil {
		user := &store.User{
			TelegramUserID: userID,
			Backend:        backend,
			Model:          ai.DefaultModel(ai.Backend(backend)),
			Mode:           store.ModeCLI,
			AIEnabled:      true,
		}
		if err := b.store.SaveUser(user); err != nil {
			b.logger.Printf("Failed to save user: %v", err)
		}
	}

	// Update in-memory state
	b.ai.setBackend(userID, backend)
	b.ai.setConnectionMode(userID, store.ModeCLI)
	b.ai.setEnabled(userID, true)

	b.sendMarkdown(chatID, fmt.Sprintf(
		"*AI configured*\n\n*Backend:* %s\n*Mode:* CLI (local)\n*Binary:* `%s`\n\nJust type a message to chat with the AI. Use /ai off to disable, /settings to change.",
		backendDisplayName(backend), escapeMarkdown(path)))
}

// handleOnboardingInput checks if the user is in the middle of onboarding
// (awaiting API key or model input) and processes accordingly.
// Returns true if the message was handled.
func (b *Bot) handleOnboardingInput(msg *tgbotapi.Message) bool {
	userID := msg.From.ID
	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)

	// Check if awaiting API key
	if backend, ok := b.onboarding.getAwaitingKey(userID); ok {
		// CRITICAL: Delete the message containing the API key immediately
		b.deleteMessage(chatID, msg.MessageID)
		b.sendMarkdown(chatID, "API key stored securely. Message deleted for safety.")

		b.onboarding.clearAwaitingKey(userID)

		// Validate the key format (basic check)
		if len(text) < 10 {
			b.sendMarkdown(chatID, "That does not look like a valid API key. Please try /settings and set up again.")
			return true
		}

		// Encrypt and save
		encKey, err := b.store.EncryptKey(text)
		if err != nil {
			b.sendMarkdown(chatID, "Failed to encrypt API key. Please try again.")
			return true
		}

		u := &store.User{
			TelegramUserID: userID,
			Backend:        backend,
			APIKeyEnc:      encKey,
			Model:          ai.DefaultModel(ai.Backend(backend)),
			Mode:           store.ModeAPI,
			AIEnabled:      true,
		}
		if err := b.store.SaveUser(u); err != nil {
			b.sendMarkdown(chatID, fmt.Sprintf("Failed to save settings: %s", err))
			return true
		}

		// Update in-memory state
		b.ai.setBackend(userID, backend)
		b.ai.setConnectionMode(userID, store.ModeAPI)

		b.sendMarkdown(chatID, fmt.Sprintf(
			"*%s configured.*\n\nAPI key saved (encrypted).\nModel: `%s`\nMode: API Key\nAI mode: *enabled*\n\nJust type your message to chat. Use /settings to manage.",
			backendDisplayName(backend), ai.DefaultModel(ai.Backend(backend))))
		return true
	}

	// Check if awaiting model name
	if b.onboarding.getAwaitingModel(userID) {
		b.onboarding.clearAwaitingModel(userID)

		if err := b.store.UpdateModel(userID, text); err != nil {
			b.sendMarkdown(chatID, fmt.Sprintf("Failed to update model: %s", err))
			return true
		}

		b.sendMarkdown(chatID, fmt.Sprintf("Model updated to `%s`.", escapeMarkdown(text)))
		return true
	}

	return false
}

// handleAPIAIMessage routes a text message to the user's configured AI backend
// when they have API mode configured. Returns true if the message was handled.
func (b *Bot) handleAPIAIMessage(msg *tgbotapi.Message) bool {
	user, err := b.store.GetUser(msg.From.ID)
	if err != nil || user == nil {
		return false
	}

	if !user.AIEnabled {
		return false
	}

	// Only handle API mode here; CLI mode is handled by the legacy ai_mode path
	if user.Mode == store.ModeCLI {
		return false
	}

	// For API mode, need an API key (unless Ollama)
	if user.APIKeyEnc == "" && user.Backend != BackendOllama {
		return false
	}

	chatID := msg.Chat.ID

	// Check daily usage limit
	if !b.checkAndIncrementUsage(chatID, msg.From.ID) {
		return true
	}

	prompt := msg.Text

	// Get the API key
	var apiKey string
	if user.Backend != BackendOllama {
		apiKey, err = b.store.DecryptKey(user.APIKeyEnc)
		if err != nil {
			b.sendMarkdown(chatID, "Failed to decrypt API key. Please reconfigure with /settings.")
			return true
		}
	}

	// Create client
	client, err := ai.NewClient(ai.Backend(user.Backend), apiKey)
	if err != nil {
		b.sendMarkdown(chatID, fmt.Sprintf("Backend error: %s", err))
		return true
	}

	b.sendMarkdown(chatID, fmt.Sprintf("_Asking %s..._", user.Backend))

	ctx, cancel := context.WithTimeout(context.Background(), ai.DefaultTimeout)
	defer cancel()

	response, err := client.Chat(ctx, prompt, user.Model)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			b.sendMarkdown(chatID, fmt.Sprintf("%s timed out after 60s.", user.Backend))
			return true
		}
		b.sendMarkdown(chatID, fmt.Sprintf("%s error: %s", user.Backend, escapeMarkdown(err.Error())))
		return true
	}

	if response == "" {
		b.sendMarkdown(chatID, fmt.Sprintf("_%s returned an empty response._", user.Backend))
		return true
	}

	// PII filter
	response = b.filter.Sanitize(response)

	// Chunk and send
	chunks := chunkText(response, maxTelegramMessage-100)
	for _, chunk := range chunks {
		b.sendMarkdown(chatID, chunk)
	}

	return true
}

// handleSettingsCommand shows current AI settings and allows changes.
func (b *Bot) handleSettingsCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	userID := msg.From.ID

	if b.store == nil {
		b.sendMarkdown(chatID, "Store not configured.")
		return
	}

	user, err := b.store.GetUser(userID)
	if err != nil {
		b.sendMarkdown(chatID, fmt.Sprintf("Error reading settings: %s", err))
		return
	}

	if user == nil {
		b.sendMarkdown(chatID, "No AI settings configured yet. Use /onboard to get started.")
		return
	}

	mode := user.Mode
	if mode == "" {
		mode = store.ModeCLI
	}

	hasKey := "no"
	if user.APIKeyEnc != "" {
		hasKey = "yes (encrypted)"
	}

	text := fmt.Sprintf("*AI Settings*\n\n*Backend:* %s\n*Mode:* %s\n*API Key:* %s\n*AI Enabled:* %v\n\nUse the buttons below to change settings, or /onboard to reconfigure.",
		escapeMarkdown(user.Backend),
		escapeMarkdown(mode),
		hasKey,
		user.AIEnabled)

	m := tgbotapi.NewMessage(chatID, text)
	m.ParseMode = "Markdown"
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Change Backend", "settings:show:backend"),
			tgbotapi.NewInlineKeyboardButtonData("Change Mode", "settings:show:mode"),
		),
	)
	b.api.Send(m)
}

// handleSettingsCallback processes callbacks from /settings inline keyboards.
func (b *Bot) handleSettingsCallback(cb *tgbotapi.CallbackQuery) {
	chatID := cb.Message.Chat.ID
	userID := cb.From.ID

	data := strings.TrimPrefix(cb.Data, "settings:")
	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return
	}
	action := parts[0]
	value := parts[1]

	if b.store == nil {
		b.sendMarkdown(chatID, "Store not configured.")
		return
	}

	switch action {
	case "show":
		if value == "mode" {
			m := tgbotapi.NewMessage(chatID, "Select connection mode:")
			m.ReplyMarkup = settingsModeKeyboard()
			b.api.Send(m)
		} else if value == "backend" {
			m := tgbotapi.NewMessage(chatID, "Select AI backend:")
			m.ReplyMarkup = settingsBackendKeyboard()
			b.api.Send(m)
		}

	case "mode":
		if value == "cli" {
			user, err := b.store.GetUser(userID)
			if err != nil || user == nil {
				b.sendMarkdown(chatID, "No user record found. Run /onboard first.")
				return
			}
			binary := store.CLIBinaryForBackend(user.Backend)
			if _, err := exec.LookPath(binary); err != nil {
				b.sendMarkdown(chatID, fmt.Sprintf(
					"`%s` CLI not found on this system. Install it first, or use API Key mode.",
					binary))
				return
			}
			if err := b.store.UpdateMode(userID, store.ModeCLI); err != nil {
				b.sendMarkdown(chatID, fmt.Sprintf("Error updating mode: %s", err))
				return
			}
			b.ai.setConnectionMode(userID, store.ModeCLI)
			b.sendMarkdown(chatID, "Mode switched to *CLI (local)*. The bot will use the local CLI binary.")
		} else if value == "api" {
			if err := b.store.UpdateMode(userID, store.ModeAPI); err != nil {
				b.sendMarkdown(chatID, fmt.Sprintf("Error updating mode: %s", err))
				return
			}
			b.ai.setConnectionMode(userID, store.ModeAPI)
			b.sendMarkdown(chatID, "Mode switched to *API Key*. Use /onboard to set a new key if needed.")
		}

	case "backend":
		if err := b.store.UpdateBackend(userID, value); err != nil {
			b.sendMarkdown(chatID, fmt.Sprintf("Error updating backend: %s", err))
			return
		}
		b.ai.setBackend(userID, value)
		b.sendMarkdown(chatID, fmt.Sprintf("Backend switched to *%s*.", backendDisplayName(value)))
	}
}

// handleOnboardCommand starts the AI onboarding flow.
func (b *Bot) handleOnboardCommand(msg *tgbotapi.Message) {
	b.sendOnboardingPrompt(msg.Chat.ID)
}

// backendDisplayName returns a display-friendly name for a backend.
func backendDisplayName(backend string) string {
	switch backend {
	case "claude":
		return "Claude"
	case "openai":
		return "OpenAI"
	case "gemini":
		return "Gemini"
	case "ollama":
		return "Ollama"
	default:
		return backend
	}
}
