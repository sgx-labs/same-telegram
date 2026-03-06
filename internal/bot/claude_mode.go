package bot

import (
	"fmt"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/audit"
	"github.com/sgx-labs/same-telegram/internal/exec"
)

// maxTelegramMessage is Telegram's max message length.
const maxTelegramMessage = 4096

// claudeToggle tracks per-user Claude mode state.
type claudeToggle struct {
	mu    sync.RWMutex
	users map[int64]bool
}

func newClaudeToggle() *claudeToggle {
	return &claudeToggle{
		users: make(map[int64]bool),
	}
}

func (ct *claudeToggle) isEnabled(userID int64) bool {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.users[userID]
}

func (ct *claudeToggle) set(userID int64, enabled bool) {
	ct.mu.Lock()
	ct.users[userID] = enabled
	ct.mu.Unlock()
}

// handleClaudeCommand processes /claude [on|off] commands.
// Deprecated: /claude is now an alias for /ai. /claude on maps to /ai claude,
// /claude off maps to /ai off.
func (b *Bot) handleClaudeCommand(msg *tgbotapi.Message, args string) {
	args = strings.TrimSpace(strings.ToLower(args))

	switch args {
	case "on":
		b.handleAICommand(msg, "claude")
	case "off":
		b.handleAICommand(msg, "off")
	default:
		b.handleAICommand(msg, "")
	}

	b.sendMarkdown(msg.Chat.ID, "_Tip: /claude is now an alias for /ai. Use /ai to manage AI mode._")
}

// handleClaudeMessage sends a text message to the claude CLI and returns the response.
// Output is PII-filtered and chunked for Telegram's message size limit.
// Uses per-user session persistence so conversations feel continuous.
func (b *Bot) handleClaudeMessage(chatID int64, userID int64, prompt string) {
	if !b.checkAndIncrementUsage(chatID, userID) {
		return
	}

	b.sendMarkdown(chatID, "_Thinking..._")

	opts := exec.ClaudeOptions{
		DangerousPermissions: b.cfg.Bot.DangerousPermissions,
	}

	// Resume existing session if available
	if sid := b.sessions.Get(userID); sid != "" {
		opts.SessionID = sid
	}

	result, err := exec.RunClaudeWithSession(prompt, opts)
	if err != nil {
		errMsg := err.Error()
		// Sanitize error output — stderr may contain paths or tokens
		errMsg = b.filter.Sanitize(errMsg)
		b.sendMarkdown(chatID, fmt.Sprintf("Claude error: %s", escapeMarkdown(errMsg)))
		return
	}

	// Audit log the Claude invocation (sanitize prompt to strip PII)
	b.auditLog.Log(audit.Entry{
		UserID:               userID,
		Prompt:               b.filter.Sanitize(prompt),
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
		b.sendMarkdown(chatID, "_Claude returned an empty response._")
		return
	}

	// PII filter all output before sending to Telegram
	out = b.filter.Sanitize(out)

	// Chunk and send
	chunks := chunkText(out, maxTelegramMessage-100) // leave margin for markdown overhead
	for _, chunk := range chunks {
		b.sendMarkdown(chatID, chunk)
	}
}

// chunkText splits text into chunks of at most maxLen bytes,
// breaking at newlines when possible.
func chunkText(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxLen {
			chunks = append(chunks, text)
			break
		}

		// Try to break at a newline within the limit
		cut := maxLen
		if idx := strings.LastIndex(text[:maxLen], "\n"); idx > 0 {
			cut = idx + 1
		}

		chunks = append(chunks, text[:cut])
		text = text[cut:]
	}
	return chunks
}
