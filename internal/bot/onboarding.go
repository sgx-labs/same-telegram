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
	text := "*Welcome!*\n\n" +
		"I'm an AI assistant with persistent memory — I remember our conversations across sessions.\n\n" +
		"To get started, I need to connect to an AI provider. Pick the one you'd like to use:\n\n" +
		"*Claude* — Anthropic's AI, great for reasoning and writing\n" +
		"*OpenAI* — GPT-4o, widely used and versatile\n" +
		"*Gemini* — Google's AI, free credits available\n" +
		"*Ollama* — Free and local, runs on your own computer\n\n" +
		"_Already have an AI subscription? Run SAME on your own machine for free and use your existing keys with no extra API costs. See github.com/sgx-labs/same-telegram_\n\n" +
		"_Self-host SAME to also get vault search, health checks, and knowledge management — free forever._"
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
// Backends with stored API keys are marked with a checkmark.
func (b *Bot) settingsBackendKeyboard(userID int64) tgbotapi.InlineKeyboardMarkup {
	configured := make(map[string]bool)
	if backends, err := b.store.GetConfiguredBackends(userID); err == nil {
		for _, backend := range backends {
			configured[backend] = true
		}
	}

	label := func(name, key string) string {
		if configured[key] {
			return name + " \u2705"
		}
		return name
	}

	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label("Claude", "claude"), "settings:backend:claude"),
			tgbotapi.NewInlineKeyboardButtonData(label("OpenAI", "openai"), "settings:backend:openai"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label("Gemini", "gemini"), "settings:backend:gemini"),
			tgbotapi.NewInlineKeyboardButtonData(label("Ollama", "ollama"), "settings:backend:ollama"),
		),
	)
}

// handleOnboardingCallback processes backend selection from inline keyboard.
// After backend selection, presents mode selection (API Key vs CLI).
func (b *Bot) handleOnboardingCallback(cb *tgbotapi.CallbackQuery) {
	backend := strings.TrimPrefix(cb.Data, "onboard:")
	chatID := cb.Message.Chat.ID
	userID := cb.From.ID

	// In public mode, skip mode selection — go straight to API key setup
	if b.isPublicMode() {
		b.handleAPIKeySetup(chatID, userID, backend)
		return
	}

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

	// Defense in depth: CLI mode is not available in public mode.
	if b.isPublicMode() && mode == "cli" {
		b.sendMarkdown(cb.Message.Chat.ID, "CLI mode is not available. Use API Key mode instead.")
		return
	}
	backend := parts[2] // "claude", "openai", etc.
	userID := cb.From.ID
	chatID := cb.Message.Chat.ID

	switch mode {
	case "cli":
		b.handleCLIModeSetup(chatID, userID, backend)

	case "api":
		b.handleAPIKeySetup(chatID, userID, backend)
	}
}

// handleAPIKeySetup prompts the user for their API key (or configures Ollama directly).
func (b *Bot) handleAPIKeySetup(chatID int64, userID int64, backend string) {
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
		ollamaMsg := fmt.Sprintf(
			"*You're all set!*\n\n"+
				"*Backend:* Ollama (local)\n"+
				"*Model:* `%s`\n\n"+
				"Just type any message and I'll respond.\n\n"+
				"*Useful commands:*\n"+
				"/model — change your AI model\n"+
				"/settings — manage your setup\n"+
				"/new — start a fresh conversation\n\n"+
				"_Make sure Ollama is running on your machine._",
			ai.DefaultModel(ai.BackendOllama))
		if b.isPublicMode() {
			ollamaMsg += "\n\n*Want more?*\n" +
				"Self-host SAME to unlock vault search, health monitoring, and AI-powered answers from your own notes — free forever.\n" +
				"https://github.com/sgx-labs/same-telegram"
		}
		b.sendMarkdown(chatID, ollamaMsg)
		return
	}

	// Ask for API key
	b.onboarding.setAwaitingKey(userID, backend)

	keyPrompt := apiKeyPrompt(backend)
	b.sendMarkdown(chatID, keyPrompt)
}

// handleCLIModeSetup verifies the CLI binary exists on the system and saves the preference.
// CLI mode is restricted to the bot owner because it executes local commands.
func (b *Bot) handleCLIModeSetup(chatID int64, userID int64, backend string) {
	ownerID := b.cfg.Bot.EffectiveOwnerID()
	if ownerID == 0 || userID != ownerID {
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

		defaultModel := ai.DefaultModel(ai.Backend(backend))
		u := &store.User{
			TelegramUserID: userID,
			Backend:        backend,
			APIKeyEnc:      encKey,
			Model:          defaultModel,
			Mode:           store.ModeAPI,
			AIEnabled:      true,
		}
		if err := b.store.SaveUser(u); err != nil {
			b.sendMarkdown(chatID, fmt.Sprintf("Failed to save settings: %s", err))
			return true
		}
		// Also store in the api_keys vault for multi-backend switching.
		if err := b.store.SaveAPIKey(userID, backend, encKey, defaultModel); err != nil {
			b.logger.Printf("Warning: failed to save API key to vault: %v", err)
		}

		// Update in-memory state
		b.ai.setBackend(userID, backend)
		b.ai.setConnectionMode(userID, store.ModeAPI)

		completionMsg := fmt.Sprintf(
			"*You're all set!*\n\n"+
				"*Backend:* %s\n"+
				"*Model:* `%s`\n"+
				"API key saved (encrypted).\n\n"+
				"Just type any message and I'll respond.\n\n"+
				"*Useful commands:*\n"+
				"/model — change your AI model\n"+
				"/settings — manage your setup\n"+
				"/new — start a fresh conversation",
			backendDisplayName(backend), ai.DefaultModel(ai.Backend(backend)))
		if b.isPublicMode() {
			completionMsg += "\n\n*Want more?*\n" +
				"Self-host SAME to unlock vault search, health monitoring, and AI-powered answers from your own notes — free forever.\n" +
				"https://github.com/sgx-labs/same-telegram"
		}
		b.sendMarkdown(chatID, completionMsg)
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
	userID := msg.From.ID

	// Truncate prompt to 32KB
	const maxPromptBytes = 32768
	if len(prompt) > maxPromptBytes {
		prompt = prompt[:maxPromptBytes]
		b.sendMarkdown(chatID, "_Note: your message was truncated to 32KB._")
	}

	// Send typing indicator
	typing := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	b.api.Send(typing)

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

	// Build prompt with conversation history for multi-turn continuity
	history := b.conversations.Get(userID)
	fullPrompt := BuildPromptWithHistory(history, prompt)

	response, err := client.Chat(ctx, fullPrompt, user.Model)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			b.sendMarkdown(chatID, fmt.Sprintf("%s took too long to respond. Try again in a moment.", backendDisplayName(user.Backend)))
			return true
		}
		b.sendMarkdown(chatID, friendlyAIError(user.Backend, err))
		return true
	}

	if response == "" {
		b.sendMarkdown(chatID, fmt.Sprintf("_%s returned an empty response._", user.Backend))
		return true
	}

	// Store conversation pair for future context
	b.conversations.Add(userID, prompt, response)

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

	// Show configured backends from the vault
	configuredInfo := ""
	if configured, err := b.store.GetConfiguredBackends(userID); err == nil && len(configured) > 0 {
		var names []string
		for _, cb := range configured {
			name := backendDisplayName(cb)
			if cb == user.Backend {
				name += " (active)"
			}
			names = append(names, name)
		}
		configuredInfo = fmt.Sprintf("\n*Stored keys:* %s", strings.Join(names, ", "))
	}

	text := fmt.Sprintf("*AI Settings*\n\n*Backend:* %s\n*Mode:* %s\n*API Key:* %s\n*AI Enabled:* %v%s\n\nUse the buttons below to change settings, or /onboard to reconfigure.",
		escapeMarkdown(user.Backend),
		escapeMarkdown(mode),
		hasKey,
		user.AIEnabled,
		configuredInfo)

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
			m.ReplyMarkup = b.settingsBackendKeyboard(userID)
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
		// Check if user has a stored key for the new backend
		encKey, model, err := b.store.GetAPIKey(userID, value)
		if err != nil {
			b.sendMarkdown(chatID, fmt.Sprintf("Error checking stored keys: %s", err))
			return
		}

		if encKey != "" {
			// Seamless switch: restore key and model from vault
			if model == "" {
				model = ai.DefaultModel(ai.Backend(value))
			}
			user, _ := b.store.GetUser(userID)
			mode := store.ModeAPI
			aiEnabled := true
			if user != nil {
				mode = user.Mode
				aiEnabled = user.AIEnabled
			}
			u := &store.User{
				TelegramUserID: userID,
				Backend:        value,
				APIKeyEnc:      encKey,
				Model:          model,
				Mode:           mode,
				AIEnabled:      aiEnabled,
			}
			if err := b.store.SaveUser(u); err != nil {
				b.sendMarkdown(chatID, fmt.Sprintf("Error updating backend: %s", err))
				return
			}
			b.ai.setBackend(userID, value)
			b.sendMarkdown(chatID, fmt.Sprintf("Backend switched to *%s*. Your stored API key and model (`%s`) have been restored.", backendDisplayName(value), escapeMarkdown(model)))
		} else if value == BackendOllama {
			// Ollama doesn't need an API key
			if err := b.store.UpdateBackend(userID, value); err != nil {
				b.sendMarkdown(chatID, fmt.Sprintf("Error updating backend: %s", err))
				return
			}
			b.ai.setBackend(userID, value)
			b.sendMarkdown(chatID, fmt.Sprintf("Backend switched to *%s*.", backendDisplayName(value)))
		} else {
			// No stored key — trigger API key setup
			b.sendMarkdown(chatID, fmt.Sprintf("No API key stored for *%s*. Let's set one up.", backendDisplayName(value)))
			b.handleAPIKeySetup(chatID, userID, value)
		}
	}
}

// handleOnboardCommand starts the AI onboarding flow.
func (b *Bot) handleOnboardCommand(msg *tgbotapi.Message) {
	b.sendOnboardingPrompt(msg.Chat.ID)
}

// apiKeyPrompt returns a beginner-friendly prompt for the given backend's API key.
func apiKeyPrompt(backend string) string {
	switch backend {
	case "claude":
		return "*Claude selected*\n\n" +
			"An API key lets me talk to Claude (Anthropic's AI) on your behalf.\n\n" +
			"*How to get a key:*\n" +
			"1. Sign up or log in at https://platform.claude.com/settings/keys\n" +
			"2. Go to *API Keys*\n" +
			"3. Click *Create Key*\n" +
			"4. Copy the key and paste it here\n\n" +
			"Expected format: `sk-ant-api...`\n\n" +
			"*Cost:* Anthropic charges per message (~$0.003–0.015 per message depending on model). Most users spend $1–5/month.\n\n" +
			"Your key is encrypted and stored securely. Your message will be deleted immediately.\n\n" +
			"Send /cancel to abort."
	case "openai":
		return "*OpenAI selected*\n\n" +
			"An API key lets me talk to GPT (OpenAI's AI) on your behalf.\n\n" +
			"*How to get a key:*\n" +
			"1. Sign up or log in at https://platform.openai.com/api-keys\n" +
			"2. Go to *API Keys*\n" +
			"3. Click *Create new secret key*\n" +
			"4. Copy the key and paste it here\n\n" +
			"Expected format: `sk-proj-...`\n\n" +
			"*Cost:* OpenAI charges per message (~$0.002–0.01 per message depending on model). Most users spend $1–5/month.\n\n" +
			"Your key is encrypted and stored securely. Your message will be deleted immediately.\n\n" +
			"Send /cancel to abort."
	case "gemini":
		return "*Gemini selected*\n\n" +
			"An API key lets me talk to Gemini (Google's AI) on your behalf.\n\n" +
			"*How to get a key:*\n" +
			"1. Go to https://aistudio.google.com/apikey\n" +
			"2. Sign in with your Google account\n" +
			"3. Click *Create API Key*\n" +
			"4. Copy the key and paste it here\n\n" +
			"Expected format: `AIzaSy...`\n\n" +
			"*Cost:* Google offers free API credits to get started. Paid usage is ~$0.001–0.005 per message.\n\n" +
			"Your key is encrypted and stored securely. Your message will be deleted immediately.\n\n" +
			"Send /cancel to abort."
	default:
		return "*Please send your API key now.*\n\nSend /cancel to abort."
	}
}

// friendlyAIError translates raw AI API errors into user-friendly messages.
func friendlyAIError(backend string, err error) string {
	errStr := err.Error()
	name := backendDisplayName(backend)

	billingLinks := map[string]string{
		"claude": "https://platform.claude.com/settings/billing",
		"openai": "https://platform.openai.com/account/billing",
		"gemini": "https://aistudio.google.com/apikey",
	}

	// Connection errors
	if strings.Contains(errStr, "API call:") || strings.Contains(errStr, "dial") || strings.Contains(errStr, "connection refused") {
		if backend == "ollama" {
			return "Couldn't connect to Ollama. Make sure it's installed and running on your machine.\n\nInstall: https://ollama.com"
		}
		return fmt.Sprintf("Couldn't reach %s. Check your internet connection and try again.", name)
	}

	// Auth errors (401/403)
	if strings.Contains(errStr, "HTTP 401") || strings.Contains(errStr, "HTTP 403") {
		return fmt.Sprintf("That API key doesn't seem to be valid for %s. Double-check you copied the full key, or set a new one with /settings.", name)
	}

	// Credit/billing errors
	if strings.Contains(errStr, "HTTP 400") && (strings.Contains(errStr, "credit") || strings.Contains(errStr, "billing") || strings.Contains(errStr, "balance")) {
		link := billingLinks[backend]
		if link != "" {
			return fmt.Sprintf("Your %s account needs credits. Add them at %s and try again.", name, link)
		}
		return fmt.Sprintf("Your %s account needs credits. Check your billing settings and try again.", name)
	}

	// Rate limit (429)
	if strings.Contains(errStr, "HTTP 429") {
		return fmt.Sprintf("%s is rate-limited. Wait a moment and try again.", name)
	}

	// Generic HTTP errors — show the status but keep it friendly
	if strings.Contains(errStr, "API error (HTTP") || strings.Contains(errStr, "Ollama error (HTTP") {
		return fmt.Sprintf("%s returned an error. This is usually temporary — try again in a moment.\n\n_Details: %s_", name, escapeMarkdown(errStr))
	}

	// Fallback
	return fmt.Sprintf("%s error: %s", name, escapeMarkdown(errStr))
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
