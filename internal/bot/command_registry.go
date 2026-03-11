package bot

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// publicCommands are shown in public mode (no vault/CLI features).
var publicCommands = []tgbotapi.BotCommand{
	{Command: "start", Description: "Get started"},
	{Command: "ai", Description: "AI mode on/off/settings"},
	{Command: "onboard", Description: "Set up your AI backend"},
	{Command: "settings", Description: "Manage your settings"},
	{Command: "new", Description: "Start a new conversation"},
	{Command: "stop", Description: "Cancel in-flight request"},
	{Command: "usage", Description: "Today's AI usage"},
	{Command: "vault", Description: "Learn about vault features"},
	{Command: "help", Description: "Show all commands"},
}

// alwaysCommands are available in both internal and public modes.
var alwaysCommands = []tgbotapi.BotCommand{
	{Command: "ai", Description: "AI backend settings"},
	{Command: "reset", Description: "Clear conversation session"},
	{Command: "new", Description: "Start a new conversation"},
	{Command: "clear", Description: "Clear conversation history"},
	{Command: "onboard", Description: "Set up AI backend"},
	{Command: "settings", Description: "Manage settings"},
	{Command: "status", Description: "Vault status"},
	{Command: "doctor", Description: "Health check"},
	{Command: "search", Description: "Search vault"},
	{Command: "ask", Description: "Ask SAME a question"},
	{Command: "stop", Description: "Cancel in-flight request"},
	{Command: "usage", Description: "Today's AI usage"},
	{Command: "help", Description: "Show all commands"},
}

// internalOnlyCommands are only registered and shown in internal mode.
var internalOnlyCommands = []tgbotapi.BotCommand{
	{Command: "claude", Description: "Alias for /ai (deprecated)"},
	{Command: "team", Description: "Agent team status"},
	{Command: "decisions", Description: "Pending decisions"},
	{Command: "announce", Description: "Post admin announcement"},
	{Command: "reviews", Description: "List pending reviews"},
	{Command: "review", Description: "Read a review doc"},
	{Command: "approve", Description: "Approve a review"},
	{Command: "reject", Description: "Reject a review"},
	{Command: "task", Description: "Create or view a task"},
	{Command: "tasks", Description: "List active tasks"},
	{Command: "cancel_task", Description: "Cancel a task"},
}

// botCommands returns the full command list for internal mode (backward compat).
var botCommands = append(append([]tgbotapi.BotCommand{}, alwaysCommands...), internalOnlyCommands...)

// workspaceCommands are shown in workspace mode (minimal — just start and help).
var workspaceCommands = []tgbotapi.BotCommand{
	{Command: "start", Description: "Set up your workspace"},
	{Command: "destroy", Description: "Permanently delete your workspace"},
	{Command: "feedback", Description: "Send feedback to the team"},
	{Command: "privacy", Description: "Toggle analytics opt-out"},
	{Command: "help", Description: "Show commands"},
}

// commandsForMode returns the appropriate command list based on mode.
func commandsForMode(mode string) []tgbotapi.BotCommand {
	switch mode {
	case "workspace":
		return workspaceCommands
	case "public":
		return publicCommands
	default:
		return botCommands
	}
}

// generateHelpText builds the /help response, filtering by bot mode.
func (b *Bot) generateHelpText() string {
	// Workspace mode: short, focused help.
	if b.isWorkspaceMode() {
		return "*SameVault Commands*\n\n" +
			"/start -- Set up your workspace\n" +
			"/destroy -- Permanently delete your workspace\n" +
			"/feedback -- Send feedback to the team\n" +
			"/privacy -- Toggle analytics opt-out\n" +
			"/help -- Show this message"
	}

	var sb strings.Builder
	sb.WriteString("*SAME Telegram Bot -- Commands*\n\n")

	// Group commands by category for readability.
	type group struct {
		label    string
		commands []string
		internal bool // true = only shown in internal mode
	}

	groups := []group{
		{"AI", []string{"ai", "reset", "new", "clear", "onboard", "settings"}, false},
		{"Team", []string{"team", "decisions", "announce"}, true},
		{"Reviews", []string{"reviews", "review", "approve", "reject"}, true},
		{"Tasks", []string{"task", "tasks", "cancel_task"}, true},
		{"Management", []string{"status", "doctor", "search", "ask", "stop", "usage", "help"}, false},
	}

	isPublic := b.isPublicMode()

	// Build a lookup map from the full registry.
	desc := make(map[string]string, len(botCommands))
	for _, c := range botCommands {
		desc[c.Command] = c.Description
	}

	// In public mode, filter out internal-only commands.
	internalCmds := map[string]bool{
		"claude": true, "team": true, "decisions": true, "announce": true,
		"reviews": true, "review": true, "approve": true, "reject": true,
		"task": true, "tasks": true, "cancel_task": true,
	}

	for _, g := range groups {
		if isPublic && g.internal {
			continue
		}
		sb.WriteString(fmt.Sprintf("*%s:*\n", g.label))
		for _, cmd := range g.commands {
			if isPublic && internalCmds[cmd] {
				continue
			}
			d := desc[cmd]
			// Use hyphen form for display (Telegram menu needs underscore).
			display := strings.ReplaceAll(cmd, "_", "-")
			sb.WriteString(fmt.Sprintf("/%s -- %s\n", display, d))
		}
		sb.WriteString("\n")
	}

	// In public mode, show vault features teaser
	if isPublic {
		sb.WriteString("\n*SAME Vault Features (self-hosted only):*\n")
		sb.WriteString("Search your knowledge base, monitor vault health, get AI-powered answers from your notes, and more. These features require running SAME on your own machine.\n\n")
		sb.WriteString("Self-host guide: https://github.com/sgx-labs/same-telegram\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}
