package bot

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/analytics"
)

// logEvent is a nil-safe helper for recording analytics events.
func (b *Bot) logEvent(userID int64, eventType, value string) {
	if b.analytics != nil {
		b.analytics.Log(userID, eventType, value)
	}
}

// handleFeedbackCommand processes /feedback <message>.
func (b *Bot) handleFeedbackCommand(msg *tgbotapi.Message) {
	text := strings.TrimSpace(msg.CommandArguments())
	if text == "" {
		b.sendMarkdown(msg.Chat.ID,
			"*Send feedback*\n\n"+
				"Usage: `/feedback your message here`\n\n"+
				"Your feedback goes straight to the team. Include bugs, ideas, or anything on your mind.")
		return
	}

	b.logEvent(msg.From.ID, analytics.EventFeedback, text)
	b.logger.Printf("Feedback from user %d: %s", msg.From.ID, text)
	b.sendMarkdown(msg.Chat.ID, "Thanks for the feedback! We read every message.")
}

// handlePrivacyCommand processes /privacy — toggles analytics opt-out.
func (b *Bot) handlePrivacyCommand(msg *tgbotapi.Message) {
	if b.analytics == nil {
		b.sendMarkdown(msg.Chat.ID, "Analytics are not enabled.")
		return
	}

	userID := msg.From.ID
	if b.analytics.IsOptedOut(userID) {
		b.analytics.OptIn(userID)
		b.sendMarkdown(msg.Chat.ID,
			"*Analytics re-enabled.*\n\n"+
				"Anonymous usage events will be recorded to help improve the product. "+
				"No personal data is collected.\n\n"+
				"Tap /privacy again to opt out.")
	} else {
		b.analytics.OptOut(userID)
		b.sendMarkdown(msg.Chat.ID,
			"*Analytics disabled.*\n\n"+
				"All your existing data has been deleted and no future events will be logged.\n\n"+
				"Tap /privacy again to opt back in.")
	}
}

// handleStatsCommand processes /stats — admin-only usage dashboard.
func (b *Bot) handleStatsCommand(msg *tgbotapi.Message) {
	// Restrict to bot owner.
	ownerID := b.cfg.Bot.EffectiveOwnerID()
	if ownerID != 0 && msg.From.ID != ownerID {
		b.sendMarkdown(msg.Chat.ID, "This command is only available to the admin.")
		return
	}

	if b.analytics == nil {
		b.sendMarkdown(msg.Chat.ID, "Analytics are not enabled.")
		return
	}

	sum, err := b.analytics.GetSummary()
	if err != nil {
		b.sendMarkdown(msg.Chat.ID, fmt.Sprintf("Error loading stats: %v", err))
		return
	}

	var sb strings.Builder
	sb.WriteString("*SameVault Analytics*\n\n")
	sb.WriteString(fmt.Sprintf("Users: *%d* | Workspaces: *%d*\n", sum.TotalUsers, sum.TotalWorkspaces))
	sb.WriteString(fmt.Sprintf("Sessions: *%d* | Feedback: *%d*\n", sum.TotalSessions, sum.FeedbackCount))
	sb.WriteString(fmt.Sprintf("Active today: *%d* | This week: *%d*\n", sum.ActiveToday, sum.ActiveThisWeek))

	if len(sum.SeedBreakdown) > 0 {
		sb.WriteString("\n*Seeds:*\n")
		for seed, count := range sum.SeedBreakdown {
			sb.WriteString(fmt.Sprintf("  %s: %d\n", seed, count))
		}
	}

	if len(sum.AuthBreakdown) > 0 {
		sb.WriteString("\n*Auth methods:*\n")
		for auth, count := range sum.AuthBreakdown {
			sb.WriteString(fmt.Sprintf("  %s: %d\n", auth, count))
		}
	}

	// Invite tracking from in-memory state.
	inviteCount := b.onboarding.inviteCount()
	sb.WriteString(fmt.Sprintf("\n*Invites used:* %d / %d\n", inviteCount, maxInviteUses))

	if len(sum.RecentFeedback) > 0 {
		sb.WriteString("\n*Recent feedback:*\n")
		for _, f := range sum.RecentFeedback {
			preview := f.Message
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}
			sb.WriteString(fmt.Sprintf("  [%d] %s\n", f.UserID, preview))
		}
	}

	b.sendMarkdown(msg.Chat.ID, sb.String())
}
