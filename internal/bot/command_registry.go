package bot

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// botCommands is the single source of truth for all bot commands.
// Used both for registering with Telegram's setMyCommands and generating /help text.
var botCommands = []tgbotapi.BotCommand{
	{Command: "claude", Description: "Toggle Claude mode"},
	{Command: "ai", Description: "AI backend settings"},
	{Command: "reset", Description: "Clear conversation session"},
	{Command: "onboard", Description: "Set up AI backend"},
	{Command: "settings", Description: "Manage settings"},
	{Command: "team", Description: "Agent team status"},
	{Command: "decisions", Description: "Pending decisions"},
	{Command: "announce", Description: "Post CEO announcement"},
	{Command: "reviews", Description: "List pending reviews"},
	{Command: "review", Description: "Read a review doc"},
	{Command: "approve", Description: "Approve a review"},
	{Command: "reject", Description: "Reject a review"},
	{Command: "status", Description: "Vault status"},
	{Command: "doctor", Description: "Health check"},
	{Command: "search", Description: "Search vault"},
	{Command: "ask", Description: "Ask SAME a question"},
	{Command: "stop", Description: "Cancel in-flight request"},
	{Command: "task", Description: "Create or view a task"},
	{Command: "tasks", Description: "List active tasks"},
	{Command: "cancel_task", Description: "Cancel a task"},
	{Command: "usage", Description: "Today's AI usage"},
	{Command: "help", Description: "Show all commands"},
}

// generateHelpText builds the /help response from botCommands.
func generateHelpText() string {
	var b strings.Builder
	b.WriteString("*SAME Telegram Bot -- Commands*\n\n")

	// Group commands by category for readability.
	type group struct {
		label    string
		commands []string
	}

	groups := []group{
		{"AI", []string{"claude", "ai", "reset", "onboard", "settings"}},
		{"Team", []string{"team", "decisions", "announce"}},
		{"Reviews", []string{"reviews", "review", "approve", "reject"}},
		{"Tasks", []string{"task", "tasks", "cancel_task"}},
		{"Management", []string{"status", "doctor", "search", "ask", "stop", "usage", "help"}},
	}

	// Build a lookup map from the registry.
	desc := make(map[string]string, len(botCommands))
	for _, c := range botCommands {
		desc[c.Command] = c.Description
	}

	for _, g := range groups {
		b.WriteString(fmt.Sprintf("*%s:*\n", g.label))
		for _, cmd := range g.commands {
			d := desc[cmd]
			// Use hyphen form for display (Telegram menu needs underscore).
			display := strings.ReplaceAll(cmd, "_", "-")
			b.WriteString(fmt.Sprintf("/%s -- %s\n", display, d))
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}
