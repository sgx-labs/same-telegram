package bot

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/audit"
	sameExec "github.com/sgx-labs/same-telegram/internal/exec"
)

const aiTimeout = 60 * time.Second

// aiBackendConfig holds the CLI command and args for an AI backend.
type aiBackendConfig struct {
	Command string
	Args    []string
}

// aiBackends maps backend names to their CLI configs.
var aiBackends = map[string]aiBackendConfig{
	"claude": {Command: "claude", Args: []string{"--print", "-p"}},
	"codex":  {Command: "codex", Args: []string{"-p"}},
	"gemini": {Command: "gemini", Args: []string{"-p"}},
	"ollama": {Command: "ollama", Args: []string{"run", "qwen2.5-coder:7b"}},
}

// aiBackendDescriptions provides user-friendly descriptions for each backend.
var aiBackendDescriptions = map[string]string{
	"claude": "Claude Code (Anthropic) — best for coding tasks",
	"codex":  "Codex (OpenAI) — OpenAI's coding agent",
	"gemini": "Gemini (Google) — Google's AI model",
	"ollama": "Ollama (local) — free, private, no account needed",
}

// aiState tracks per-user AI mode, backend selection, and connection mode.
type aiState struct {
	mu             sync.RWMutex
	mode           map[int64]bool
	backend        map[int64]string
	connectionMode map[int64]string // "api" or "cli"
}

func newAIState() *aiState {
	return &aiState{
		mode:           make(map[int64]bool),
		backend:        make(map[int64]string),
		connectionMode: make(map[int64]string),
	}
}

func (s *aiState) isEnabled(userID int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode[userID]
}

func (s *aiState) setEnabled(userID int64, enabled bool) {
	s.mu.Lock()
	s.mode[userID] = enabled
	s.mu.Unlock()
}

func (s *aiState) getBackend(userID int64) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b := s.backend[userID]
	if b == "" {
		return "claude"
	}
	return b
}

func (s *aiState) setBackend(userID int64, backend string) {
	s.mu.Lock()
	s.backend[userID] = backend
	s.mu.Unlock()
}

func (s *aiState) getConnectionMode(userID int64) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.connectionMode[userID]
	if m == "" {
		return "cli" // default to CLI mode
	}
	return m
}

func (s *aiState) setConnectionMode(userID int64, mode string) {
	s.mu.Lock()
	s.connectionMode[userID] = mode
	s.mu.Unlock()
}

// handleAICommand processes /ai [on|off|claude|codex|gemini] commands.
func (b *Bot) handleAICommand(msg *tgbotapi.Message, args string) {
	args = strings.TrimSpace(strings.ToLower(args))
	userID := msg.From.ID
	chatID := msg.Chat.ID

	switch args {
	case "on":
		b.ai.setEnabled(userID, true)
		backend := b.ai.getBackend(userID)
		b.sendMarkdown(chatID, fmt.Sprintf(
			"🤖 *AI mode enabled* (backend: %s)\n\nAll text messages will be sent to %s. Use /ai off to disable.",
			backend, backend))

	case "off":
		b.ai.setEnabled(userID, false)
		b.sendMarkdown(chatID, "🔇 *AI mode disabled.*\n\nText messages will be treated as search queries.")

	case "claude", "codex", "gemini", "ollama":
		cfg := aiBackends[args]
		if _, err := exec.LookPath(cfg.Command); err != nil {
			install := map[string]string{
				"claude": "npm install -g @anthropic-ai/claude-code",
				"codex":  "npm install -g @openai/codex",
				"gemini": "npm install -g @google/gemini-cli",
				"ollama": "brew install ollama && ollama pull qwen2.5-coder:7b",
			}
			b.sendMarkdown(chatID, fmt.Sprintf(
				"*%s* not found.\n\nInstall with:\n`%s`", cfg.Command, install[args]))
			return
		}
		b.ai.setBackend(userID, args)
		b.ai.setEnabled(userID, true)
		desc := aiBackendDescriptions[args]
		b.sendMarkdown(chatID, fmt.Sprintf(
			"*AI mode ON* -- switched to *%s*\n_%s_\n\nJust type your message. Use /ai off to disable.", args, desc))

	case "":
		enabled := b.ai.isEnabled(userID)
		if enabled {
			backend := b.ai.getBackend(userID)
			desc := aiBackendDescriptions[backend]
			b.sendMarkdown(chatID, fmt.Sprintf(
				"*AI mode: ON*\n*Backend:* %s -- %s\n\nJust type your message, or switch:\n/ai off -- disable\n/ai claude -- Anthropic Claude\n/ai codex -- OpenAI Codex\n/ai gemini -- Google Gemini\n/ai ollama -- local model (free, private)",
				backend, desc))
		} else {
			b.sendMarkdown(chatID, "*AI Mode*\n\nChat with an AI directly from Telegram.\n\n*Cloud backends (need an account):*\n/ai claude -- Claude Code by Anthropic\n/ai codex -- Codex by OpenAI\n/ai gemini -- Gemini by Google\n\n*Local (free, no account needed):*\n/ai ollama -- runs on your Mac via Ollama\nRecommended: qwen2.5-coder:7b (fast, good at code)\n\nPick a backend to get started, or:\n/ai on -- start with Claude (default)")
		}

	default:
		b.sendMarkdown(chatID, "Unknown option. Use /ai claude, /ai codex, /ai gemini, /ai ollama, /ai on, /ai off.")
	}
}

// handleAIMessage sends a text message to the selected AI backend and returns the response.
// It dispatches based on the user's connection mode: CLI shells out to the local binary,
// API mode would use an HTTP API client (currently falls back to CLI).
func (b *Bot) handleAIMessage(chatID int64, userID int64, prompt string) {
	if !b.checkAndIncrementUsage(chatID, userID) {
		return
	}

	backend := b.ai.getBackend(userID)
	connMode := b.ai.getConnectionMode(userID)

	if connMode == "api" {
		b.handleAIMessageAPI(chatID, userID, backend, prompt)
		return
	}

	// CLI mode: shell out to the local binary
	b.handleAIMessageCLI(chatID, userID, backend, prompt)
}

// handleAIMessageCLI dispatches a prompt via the local CLI binary.
// CLI mode is restricted to the bot owner because it executes local commands.
func (b *Bot) handleAIMessageCLI(chatID int64, userID int64, backend, prompt string) {
	ownerID := b.cfg.Bot.EffectiveOwnerID()
	if ownerID != 0 && userID != ownerID {
		b.sendMarkdown(chatID, "CLI mode is only available to the bot owner. Please use API key mode instead — run /onboard or /settings to configure.")
		return
	}

	cfg, ok := aiBackends[backend]
	if !ok {
		// Map onboarding backend names to aiBackends keys
		mapped := map[string]string{
			"openai": "codex",
		}
		if alt, found := mapped[backend]; found {
			cfg, ok = aiBackends[alt]
		}
		if !ok {
			b.sendMarkdown(chatID, fmt.Sprintf("Unknown AI backend: %s", backend))
			return
		}
	}

	// Verify CLI exists
	if _, err := exec.LookPath(cfg.Command); err != nil {
		b.sendMarkdown(chatID, fmt.Sprintf(
			"`%s` CLI not found. Please install it, or switch mode with /settings.",
			cfg.Command))
		return
	}

	b.sendMarkdown(chatID, fmt.Sprintf("_Asking %s..._", backend))

	// For Claude backend, use RunClaudeWithSession for MCP + session support
	if backend == "claude" {
		opts := sameExec.ClaudeOptions{
			DangerousPermissions: b.cfg.Bot.DangerousPermissions,
		}
		if sid := b.sessions.Get(userID); sid != "" {
			opts.SessionID = sid
		}

		result, err := sameExec.RunClaudeWithSession(prompt, opts)
		if err != nil {
			errMsg := b.filter.Sanitize(err.Error())
			b.sendMarkdown(chatID, fmt.Sprintf("%s error: %s", backend, escapeMarkdown(errMsg)))
			return
		}

		// Audit log the Claude invocation
		b.auditLog.Log(audit.Entry{
			UserID:               userID,
			Prompt:               prompt,
			Response:             result.Text,
			SessionID:            result.SessionID,
			DangerousPermissions: opts.DangerousPermissions,
		})

		// Store session ID for future messages
		if result.SessionID != "" {
			b.sessions.Set(userID, result.SessionID)
		}

		out := strings.TrimSpace(result.Text)
		if out == "" {
			b.sendMarkdown(chatID, fmt.Sprintf("_%s returned an empty response._", backend))
			return
		}
		out = b.filter.Sanitize(out)
		chunks := chunkText(out, maxTelegramMessage-100)
		for _, chunk := range chunks {
			b.sendMarkdown(chatID, chunk)
		}
		return
	}

	// Non-Claude backends: shell out directly
	ctx, cancel := context.WithTimeout(context.Background(), aiTimeout)
	defer cancel()

	args := append(cfg.Args, prompt)
	cmd := exec.CommandContext(ctx, cfg.Command, args...)
	// Strip CLAUDECODE env var so the subprocess doesn't think it's nested.
	cmd.Env = filterEnvVars(os.Environ(), "CLAUDECODE")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			b.sendMarkdown(chatID, fmt.Sprintf("%s timed out after %s.", backend, aiTimeout))
			return
		}
		b.sendMarkdown(chatID, fmt.Sprintf("%s error: %s", backend, escapeMarkdown(err.Error())))
		return
	}

	out := strings.TrimSpace(stdout.String())
	if out == "" {
		b.sendMarkdown(chatID, fmt.Sprintf("_%s returned an empty response._", backend))
		return
	}

	// PII filter all output
	out = b.filter.Sanitize(out)

	// Chunk responses over 4096 chars
	chunks := chunkText(out, maxTelegramMessage-100)
	for _, chunk := range chunks {
		b.sendMarkdown(chatID, chunk)
	}
}

// handleAIMessageAPI dispatches a prompt via the AI API using the stored key.
func (b *Bot) handleAIMessageAPI(chatID int64, userID int64, backend, prompt string) {
	if b.store == nil {
		b.handleAIMessageCLI(chatID, userID, backend, prompt)
		return
	}

	user, err := b.store.GetUser(userID)
	if err != nil || user == nil || user.APIKeyEnc == "" {
		b.sendMarkdown(chatID, "No API key configured. Falling back to CLI mode.")
		b.handleAIMessageCLI(chatID, userID, backend, prompt)
		return
	}

	apiKey, err := b.store.DecryptKey(user.APIKeyEnc)
	if err != nil {
		b.sendMarkdown(chatID, "Failed to decrypt API key. Please reconfigure with /onboard.")
		return
	}

	client, err := newAPIClient(backend, apiKey)
	if err != nil {
		b.sendMarkdown(chatID, fmt.Sprintf("Backend error: %s", err))
		return
	}

	b.sendMarkdown(chatID, fmt.Sprintf("_Asking %s..._", backendDisplayName(backend)))

	ctx, cancel := context.WithTimeout(context.Background(), aiTimeout)
	defer cancel()

	model := user.Model
	if model == "" {
		model = defaultModelForBackend(backend)
	}

	response, err := client.Chat(ctx, prompt, model)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			b.sendMarkdown(chatID, fmt.Sprintf("%s timed out after %s.", backendDisplayName(backend), aiTimeout))
			return
		}
		b.sendMarkdown(chatID, fmt.Sprintf("%s error: %s", backendDisplayName(backend), escapeMarkdown(err.Error())))
		return
	}

	if response == "" {
		b.sendMarkdown(chatID, fmt.Sprintf("_%s returned an empty response._", backendDisplayName(backend)))
		return
	}

	// PII filter
	response = b.filter.Sanitize(response)

	// Chunk and send
	chunks := chunkText(response, maxTelegramMessage-100)
	for _, chunk := range chunks {
		b.sendMarkdown(chatID, chunk)
	}
}

// filterEnvVars returns a copy of env with variables matching any excluded key removed.
func filterEnvVars(env []string, exclude ...string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		name, _, _ := strings.Cut(e, "=")
		skip := false
		for _, ex := range exclude {
			if name == ex {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, e)
		}
	}
	return out
}
