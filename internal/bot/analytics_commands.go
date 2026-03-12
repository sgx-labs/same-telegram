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

// trackEvent is a nil-safe, non-blocking helper for recording analytics events
// with structured properties. Errors are logged but never fatal.
func (b *Bot) trackEvent(userID int64, eventType string, props map[string]string) {
	if b.analytics == nil {
		return
	}
	go func() {
		b.analytics.Track(userID, eventType, props)
	}()
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

// handleAnalyticsCommand processes /analytics — admin-only detailed usage dashboard.
// Shows total users, sessions, avg session duration, popular commands, and more.
func (b *Bot) handleAnalyticsCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	// Admin-only — same pattern as /update and /machines.
	ownerID := b.cfg.Bot.EffectiveOwnerID()
	if ownerID != 0 && msg.From.ID != ownerID {
		b.sendMarkdown(chatID, "This command is only available to the admin.")
		return
	}

	if b.analytics == nil {
		b.sendMarkdown(chatID, "Analytics are not enabled.")
		return
	}

	sum, err := b.analytics.GetSummary()
	if err != nil {
		b.sendMarkdown(chatID, fmt.Sprintf("Error loading analytics: %v", err))
		return
	}

	var sb strings.Builder
	sb.WriteString("*Usage Analytics*\n\n")

	// Overview.
	sb.WriteString("*Overview:*\n")
	sb.WriteString(fmt.Sprintf("  Total users: *%d*\n", sum.TotalUsers))
	sb.WriteString(fmt.Sprintf("  Workspaces created: *%d*\n", sum.TotalWorkspaces))
	sb.WriteString(fmt.Sprintf("  Terminal connections: *%d*\n", sum.TotalConnections))
	sb.WriteString(fmt.Sprintf("  API keys configured: *%d*\n", sum.TotalAPIKeys))
	sb.WriteString(fmt.Sprintf("  Active today: *%d*\n", sum.ActiveToday))
	sb.WriteString(fmt.Sprintf("  Active this week: *%d*\n", sum.ActiveThisWeek))

	// Sessions.
	sb.WriteString("\n*Sessions:*\n")
	sb.WriteString(fmt.Sprintf("  Total sessions: *%d*\n", sum.TotalSessions))
	if sum.AvgSessionDuration > 0 {
		avgMin := sum.AvgSessionDuration / 60.0
		if avgMin >= 1 {
			sb.WriteString(fmt.Sprintf("  Avg session duration: *%.1f min*\n", avgMin))
		} else {
			sb.WriteString(fmt.Sprintf("  Avg session duration: *%.0f sec*\n", sum.AvgSessionDuration))
		}
	}

	// Popular commands.
	if len(sum.PopularCommands) > 0 {
		sb.WriteString("\n*Popular commands:*\n")
		for cmd, count := range sum.PopularCommands {
			sb.WriteString(fmt.Sprintf("  /%s: %d\n", cmd, count))
		}
	}

	// Seed breakdown.
	if len(sum.SeedBreakdown) > 0 {
		sb.WriteString("\n*Seed templates:*\n")
		for seed, count := range sum.SeedBreakdown {
			sb.WriteString(fmt.Sprintf("  %s: %d\n", seed, count))
		}
	}

	// Auth breakdown.
	if len(sum.AuthBreakdown) > 0 {
		sb.WriteString("\n*Auth methods:*\n")
		for auth, count := range sum.AuthBreakdown {
			sb.WriteString(fmt.Sprintf("  %s: %d\n", auth, count))
		}
	}

	// Invite tracking.
	inviteCount := b.onboarding.inviteCount()
	sb.WriteString(fmt.Sprintf("\n*Invites used:* %d / %d\n", inviteCount, maxInviteUses))

	// Feedback.
	sb.WriteString(fmt.Sprintf("*Feedback messages:* %d\n", sum.FeedbackCount))
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

	b.sendMarkdown(chatID, sb.String())
}
