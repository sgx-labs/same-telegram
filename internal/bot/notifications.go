package bot

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/notify"
)

// FormatNotification converts a notification into a Telegram message.
func FormatNotification(n *notify.Notification) (string, *tgbotapi.InlineKeyboardMarkup) {
	switch n.Type {
	case notify.TypeSessionEnd:
		return formatSessionEnd(n), nil

	case notify.TypeDecision:
		kb := ApproveDecisionKeyboard(n.SessionID)
		return formatDecision(n), &kb

	case notify.TypeHandoff:
		return formatHandoff(n), nil

	case notify.TypeDigestReq:
		return formatDigestRequest(n), nil

	default:
		return formatCustom(n), nil
	}
}

func formatSessionEnd(n *notify.Notification) string {
	var b strings.Builder
	b.WriteString("🏁 *Session Ended*\n\n")
	if n.Summary != "" {
		b.WriteString(escapeMarkdown(n.Summary))
		b.WriteString("\n")
	}
	if n.Details != "" {
		b.WriteString("\n")
		b.WriteString(escapeMarkdown(n.Details))
	}
	if n.SessionID != "" {
		b.WriteString(fmt.Sprintf("\n\n_Session: %s_", escapeMarkdown(n.SessionID)))
	}
	return b.String()
}

func formatDecision(n *notify.Notification) string {
	var b strings.Builder
	b.WriteString("📋 *Decision Logged*\n\n")
	if n.Summary != "" {
		b.WriteString(escapeMarkdown(n.Summary))
	}
	if n.Details != "" {
		b.WriteString("\n\n")
		b.WriteString(escapeMarkdown(n.Details))
	}
	return b.String()
}

func formatHandoff(n *notify.Notification) string {
	var b strings.Builder
	b.WriteString("🔄 *Agent Handoff*\n\n")
	if n.Summary != "" {
		b.WriteString(escapeMarkdown(n.Summary))
	}
	if n.Details != "" {
		b.WriteString("\n\n```\n")
		b.WriteString(n.Details)
		b.WriteString("\n```")
	}
	return b.String()
}

func formatDigestRequest(n *notify.Notification) string {
	var b strings.Builder
	b.WriteString("📊 *Daily Digest*\n\n")
	if n.Summary != "" {
		b.WriteString(escapeMarkdown(n.Summary))
	}
	if n.Details != "" {
		b.WriteString("\n\n")
		b.WriteString(escapeMarkdown(n.Details))
	}
	return b.String()
}

func formatCustom(n *notify.Notification) string {
	var b strings.Builder
	b.WriteString("📌 *Notification*\n\n")
	if n.Summary != "" {
		b.WriteString(escapeMarkdown(n.Summary))
	}
	if n.Details != "" {
		b.WriteString("\n\n")
		b.WriteString(escapeMarkdown(n.Details))
	}
	return b.String()
}

// escapeMarkdown escapes Telegram MarkdownV1 special characters.
// Also escapes ] and \ which can confuse the parser in edge cases.
func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"`", "\\`",
	)
	return replacer.Replace(s)
}
