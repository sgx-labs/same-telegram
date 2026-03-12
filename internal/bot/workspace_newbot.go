package bot

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// newbotState tracks per-user /newbot flow progress.
type newbotState struct {
	step     string // "token", "template", "scaffold"
	botToken string // Telegram bot token from BotFather
	botName  string // bot username extracted from token validation
	template string // selected template: "assistant", "support", "moderator"
}

// tokenPattern validates a Telegram bot token: digits:alphanumeric+special.
var tokenPattern = regexp.MustCompile(`^\d{8,15}:[A-Za-z0-9_-]{30,50}$`)

// handleNewbotCommand starts the /newbot flow.
func (b *Bot) handleNewbotCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	userID := msg.From.ID
	userIDStr := strconv.FormatInt(userID, 10)

	if b.orchestrator == nil {
		b.sendMarkdown(chatID, "Workspace mode is not available.")
		return
	}

	// Check that the user has a workspace.
	existing, err := b.orchestrator.Status(context.Background(), userIDStr)
	if err != nil || existing == nil {
		b.sendMarkdown(chatID, "You need a workspace first. Use /start to create one.")
		return
	}

	// Initialize newbot flow state.
	b.onboarding.mu.Lock()
	if b.onboarding.newbotFlows == nil {
		b.onboarding.newbotFlows = make(map[int64]*newbotState)
	}
	b.onboarding.newbotFlows[userID] = &newbotState{step: "token"}
	b.onboarding.timestamps[userID] = time.Now()
	b.onboarding.mu.Unlock()

	text := "*Create a New Bot*\n\n" +
		"I'll help you create and deploy a Telegram bot with persistent AI memory.\n\n" +
		"First, go to @BotFather in Telegram and:\n" +
		"1. Send /newbot\n" +
		"2. Choose a name and username\n" +
		"3. Copy the API token\n\n" +
		"Paste your bot token here when ready:\n\n" +
		"_Send /cancel to abort._"

	b.sendMarkdown(chatID, text)
}

// handleNewbotTokenInput processes the bot token pasted by the user.
// Returns true if the message was consumed by this handler.
func (b *Bot) handleNewbotTokenInput(msg *tgbotapi.Message) bool {
	userID := msg.From.ID
	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)

	b.onboarding.mu.Lock()
	nb := b.onboarding.newbotFlows[userID]
	if nb == nil || nb.step != "token" {
		b.onboarding.mu.Unlock()
		return false
	}
	b.onboarding.mu.Unlock()

	// Validate token format.
	if !tokenPattern.MatchString(text) {
		b.sendMarkdown(chatID,
			"That doesn't look like a valid bot token.\n\n"+
				"A Telegram bot token looks like:\n"+
				"`123456789:ABCdefGHIjklMNOpqrsTUVwxyz`\n\n"+
				"Get one from @BotFather and paste it here.\n\n"+
				"_Send /cancel to abort._")
		return true
	}

	// Delete the message containing the token for security.
	b.deleteMessage(chatID, msg.MessageID)

	// Store the token.
	b.onboarding.mu.Lock()
	nb.botToken = text
	nb.step = "template"
	b.onboarding.timestamps[userID] = time.Now()
	b.onboarding.mu.Unlock()

	b.sendMarkdown(chatID, "Bot token received. (Your message was deleted for security.)")

	// Send template selection keyboard.
	b.sendNewbotTemplateSelection(chatID)
	return true
}

// sendNewbotTemplateSelection sends the template picker inline keyboard.
func (b *Bot) sendNewbotTemplateSelection(chatID int64) {
	text := "*Choose a template:*"

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Assistant — Personal AI assistant with memory", "newbot:tpl:assistant"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Support — Customer support with knowledge base", "newbot:tpl:support"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Moderator — Group chat moderator", "newbot:tpl:moderator"),
		),
	)

	m := tgbotapi.NewMessage(chatID, text)
	m.ParseMode = "Markdown"
	m.ReplyMarkup = kb
	b.api.Send(m)
}

// handleNewbotCallback processes inline keyboard presses for the /newbot flow.
func (b *Bot) handleNewbotCallback(cb *tgbotapi.CallbackQuery) {
	chatID := cb.Message.Chat.ID
	userID := cb.From.ID
	data := strings.TrimPrefix(cb.Data, "newbot:")

	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return
	}
	action := parts[0]
	value := parts[1]

	b.onboarding.mu.Lock()
	nb := b.onboarding.newbotFlows[userID]
	b.onboarding.mu.Unlock()

	if nb == nil {
		b.sendMarkdown(chatID, "Your session expired. Use /newbot to start again.")
		return
	}

	switch action {
	case "tpl":
		b.handleNewbotTemplateSelection(chatID, userID, value)
	}
}

// handleNewbotTemplateSelection processes template selection and scaffolds the bot.
func (b *Bot) handleNewbotTemplateSelection(chatID, userID int64, template string) {
	validTemplates := map[string]string{
		"assistant": "Assistant",
		"support":   "Support",
		"moderator": "Moderator",
	}

	displayName, ok := validTemplates[template]
	if !ok {
		b.sendMarkdown(chatID, "Invalid template. Use /newbot to start again.")
		return
	}

	b.onboarding.mu.Lock()
	nb := b.onboarding.newbotFlows[userID]
	if nb == nil || nb.step != "template" {
		b.onboarding.mu.Unlock()
		b.sendMarkdown(chatID, "Your session expired. Use /newbot to start again.")
		return
	}
	nb.template = template
	nb.step = "scaffold"
	botToken := nb.botToken
	b.onboarding.mu.Unlock()

	b.sendMarkdown(chatID, fmt.Sprintf("*%s* template selected. Scaffolding your bot...", displayName))

	// Derive a bot directory name from the template (simple default).
	botDir := fmt.Sprintf("my-%s-bot", template)

	// Scaffold in the user's workspace.
	b.scaffoldNewbot(chatID, userID, botDir, template, botToken)
}

// scaffoldNewbot executes the scaffolding commands in the user's workspace.
func (b *Bot) scaffoldNewbot(chatID, userID int64, botDir, template, botToken string) {
	userIDStr := strconv.FormatInt(userID, 10)

	if b.orchestrator == nil {
		b.sendMarkdown(chatID, "Workspace is not available. Try again later.")
		b.clearNewbotState(userID)
		return
	}

	// Build the scaffold script.
	// Creates the bot directory, copies template files, writes .env.
	script := fmt.Sprintf(
		`set -e
mkdir -p ~/bots/%s
if [ -d /workspace/templates/%s ]; then
  cp -r /workspace/templates/%s/* ~/bots/%s/ 2>/dev/null || true
  cp -r /workspace/templates/%s/.* ~/bots/%s/ 2>/dev/null || true
fi
cat > ~/bots/%s/.env << 'ENVEOF'
TELEGRAM_BOT_TOKEN=%s
ENVEOF
chmod 600 ~/bots/%s/.env
echo "ok"`,
		botDir,
		template, template, botDir,
		template, botDir,
		botDir, botToken,
		botDir,
	)

	cmd := []string{"bash", "-c", script}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := b.orchestrator.ExecInWorkspace(ctx, userIDStr, cmd)
		if err != nil {
			b.logger.Printf("newbot scaffold failed for user %d: %v", userID, err)
			b.sendMarkdown(chatID,
				fmt.Sprintf("Scaffolding failed: %v\n\nYou can set up the bot manually in your terminal.", err))
			b.clearNewbotState(userID)
			return
		}

		// Send success message.
		// SECURITY: Never echo the bot token back in chat messages.
		// The token is stored in ~/bots/<dir>/.env inside the workspace.
		successMsg := fmt.Sprintf(
			"*Bot scaffolded!*\n\n"+
				"Open your terminal and run:\n"+
				"```\n"+
				"cd ~/bots/%s\n"+
				"cat README.md\n"+
				"```\n\n"+
				"When ready to deploy:\n"+
				"```\n"+
				"fly launch\n"+
				"fly secrets set TELEGRAM_BOT_TOKEN=$(cat ~/bots/%s/.env | grep TELEGRAM_BOT_TOKEN | cut -d= -f2)\n"+
				"fly deploy\n"+
				"```\n\n"+
				"Your bot token is saved in `~/bots/%s/.env`",
			botDir,
			botDir,
			botDir,
		)

		b.sendMarkdown(chatID, successMsg)
		b.clearNewbotState(userID)
	}()
}

// clearNewbotState removes the newbot flow state for a user.
func (b *Bot) clearNewbotState(userID int64) {
	b.onboarding.mu.Lock()
	delete(b.onboarding.newbotFlows, userID)
	b.onboarding.mu.Unlock()
}
