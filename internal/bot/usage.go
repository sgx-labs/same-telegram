package bot

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// checkAndIncrementUsage checks the daily message limit and increments the counter.
// Returns true if the message is allowed, false if the limit has been reached.
func (b *Bot) checkAndIncrementUsage(chatID, userID int64) bool {
	if b.store == nil {
		return true
	}

	limit := b.cfg.AI.DailyLimit
	if limit == 0 {
		// Unlimited — still track usage but don't block
		b.store.IncrementMessageCount(userID)
		return true
	}

	count, err := b.store.GetMessageCount(userID)
	if err != nil {
		b.logger.Printf("Failed to get message count for %d: %v", userID, err)
		return true // fail open
	}

	if count >= limit {
		b.sendMarkdown(chatID, fmt.Sprintf(
			"Daily AI message limit reached (%d/%d).\nLimit resets at midnight UTC. Use /usage to check your usage.",
			count, limit))
		return false
	}

	_, err = b.store.IncrementMessageCount(userID)
	if err != nil {
		b.logger.Printf("Failed to increment message count for %d: %v", userID, err)
	}
	return true
}

// handleUsageCommand shows the user's current daily message count and limit.
func (b *Bot) handleUsageCommand(msg *tgbotapi.Message) {
	userID := msg.From.ID
	chatID := msg.Chat.ID

	if b.store == nil {
		b.sendMarkdown(chatID, "Usage tracking unavailable.")
		return
	}

	count, err := b.store.GetMessageCount(userID)
	if err != nil {
		b.sendMarkdown(chatID, fmt.Sprintf("Error reading usage: %s", err))
		return
	}

	limit := b.cfg.AI.DailyLimit
	var limitStr string
	if limit == 0 {
		limitStr = "unlimited"
	} else {
		limitStr = fmt.Sprintf("%d", limit)
	}

	b.sendMarkdown(chatID, fmt.Sprintf(
		"*Daily AI Usage*\n\nMessages today: *%d* / %s\nResets at midnight UTC.",
		count, limitStr))
}
