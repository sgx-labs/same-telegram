package bot

import (
	"fmt"
	"path/filepath"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// SendReviewNotification sends a file-watch notification to all allowed users.
// If isDecision is true, an Approve/Reject inline keyboard is included.
func (b *Bot) SendReviewNotification(category, filename, summary string, isDecision, isReply bool) {
	text := formatReviewNotification(category, filename, summary, isReply)

	var kb *tgbotapi.InlineKeyboardMarkup
	if isDecision {
		markup := DecisionReviewKeyboard(filename)
		kb = &markup
	}

	for userID := range b.allowedUsers {
		msg := tgbotapi.NewMessage(userID, text)
		msg.ParseMode = "Markdown"
		if kb != nil {
			msg.ReplyMarkup = kb
		}
		if _, err := b.api.Send(msg); err != nil {
			b.logger.Printf("Failed to send review notification to %d: %v", userID, err)
		}
	}
}

// DecisionReviewKeyboard creates Approve/Reject buttons for a decision file.
func DecisionReviewKeyboard(filename string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Approve", "review_approve:"+filename),
			tgbotapi.NewInlineKeyboardButtonData("Reject", "review_reject:"+filename),
		),
	)
}

func formatReviewNotification(category, filename, summary string, isReply bool) string {
	emoji := "📄"
	switch strings.ToLower(category) {
	case "review":
		emoji = "📋"
	case "decision":
		emoji = "⚖️"
	case "report":
		emoji = "📊"
	case "reply":
		emoji = "💬"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s *New %s*\n\n", emoji, category))
	b.WriteString(fmt.Sprintf("*File:* `%s`\n\n", filename))

	// Preview: first 200 chars
	preview := summary
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	b.WriteString(escapeMarkdown(preview))

	// Command instructions
	b.WriteString("\n\n")
	switch strings.ToLower(category) {
	case "decision":
		b.WriteString("_Use /decisions to view_")
	case "reply":
		b.WriteString("_Use /messages to view replies_")
	default:
		name := strings.TrimSuffix(filename, filepath.Ext(filename))
		b.WriteString(fmt.Sprintf("_Use /review %s to read_", escapeMarkdown(name)))
	}

	return b.String()
}
