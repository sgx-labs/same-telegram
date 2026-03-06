package bot

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/exec"
)

// handleCommand dispatches Telegram bot commands.
func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	cmd := msg.Command()
	args := msg.CommandArguments()

	var reply string
	var err error

	// internalOnly is a set of commands restricted to internal mode.
	internalOnly := map[string]bool{
		"reviews": true, "review": true, "approve": true, "reject": true,
		"decisions": true, "team": true, "announce": true,
		"task": true, "tasks": true, "cancel-task": true,
		"claude": true, "config": true,
	}

	if b.isPublicMode() && internalOnly[cmd] {
		b.sendMarkdown(msg.Chat.ID, "This command is not available.")
		return
	}

	switch cmd {
	case "start":
		if b.isPublicMode() {
			b.sendOnboardingPrompt(msg.Chat.ID)
			return
		}
		reply = startText()
	case "help":
		reply = b.helpText()
	case "status":
		reply, err = cmdStatus()
	case "doctor":
		reply, err = cmdDoctor()
	case "search":
		reply, err = cmdSearch(args)
	case "ask":
		reply, err = cmdAsk(args)
	case "vault":
		if b.isPublicMode() {
			reply = "*SAME Vault Features*\n\n" +
				"When you self-host SAME, you get:\n" +
				"- /search — find anything in your knowledge base\n" +
				"- /ask — AI-powered answers from your notes\n" +
				"- /status — monitor vault health\n" +
				"- /doctor — run health checks\n" +
				"- /vaults — manage multiple vaults\n" +
				"- /digest — daily summaries\n\n" +
				"These features are free and run entirely on your machine.\n\n" +
				"Get started: https://github.com/sgx-labs/same-telegram"
		} else {
			reply, err = cmdVaults()
		}
	case "vaults":
		reply, err = cmdVaults()
	case "digest":
		reply, err = cmdDigest()
	case "claude":
		b.handleClaudeCommand(msg, args)
		return
	case "ai":
		b.handleAICommand(msg, args)
		return
	case "onboard":
		b.handleOnboardCommand(msg)
		return
	case "settings":
		b.handleSettingsCommand(msg)
		return
	case "reset", "new", "clear":
		b.sessions.Clear(msg.From.ID)
		b.conversations.Clear(msg.From.ID)
		reply = "Session cleared. Next message starts a fresh conversation."
	case "cancel":
		b.onboarding.clear(msg.From.ID)
		reply = "Cancelled."
	case "stop":
		b.handleStop(msg)
		return
	case "config":
		reply = cmdConfig(b)
	case "team":
		reply, err = cmdTeam()
	case "decisions":
		text, decisions, derr := cmdDecisions()
		if derr != nil {
			reply = fmt.Sprintf("Error: %s", derr)
		} else {
			reply = text
			if len(decisions) > 0 {
				b.sendMarkdown(msg.Chat.ID, reply)
				for _, d := range decisions {
					kb := DecisionKeyboard(d.Filename)
					kbMsg := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("Action for: %s", escapeMarkdown(d.Filename)))
					kbMsg.ParseMode = "Markdown"
					kbMsg.ReplyMarkup = kb
					b.api.Send(kbMsg)
				}
			} else {
				b.sendMarkdown(msg.Chat.ID, reply)
			}
			// Also show decisions.md if it exists
			if msgs, ferr := cmdDecisionsFile(); ferr == nil {
				for _, m := range msgs {
					b.sendMarkdown(msg.Chat.ID, m)
				}
			}
			return
		}
	case "announce":
		reply, err = cmdAnnounce(args)
	case "reviews":
		reply, err = cmdReviews()
	case "review":
		if strings.TrimSpace(args) == "" {
			// No argument — show list (same as /reviews)
			reply, err = cmdReviews()
		} else {
			msgs, rerr := cmdReview(args)
			if rerr != nil {
				b.sendMarkdown(msg.Chat.ID, fmt.Sprintf("Error: %s", rerr))
			} else {
				for _, m := range msgs {
					b.sendMarkdown(msg.Chat.ID, m)
				}
			}
			return
		}
	case "approve":
		reply, err = cmdApproveReview(args)
	case "reject":
		reply, err = cmdRejectReview(args)
	case "task":
		b.handleTaskCommand(msg, args)
		return
	case "tasks":
		reply, err = cmdListTasks()
	case "usage":
		b.handleUsageCommand(msg)
		return
	case "cancel-task":
		reply, err = cmdCancelTask(args)
	default:
		reply = fmt.Sprintf("Unknown command: /%s\nUse /help for available commands.", cmd)
	}

	if err != nil {
		reply = fmt.Sprintf("Error: %s", err)
	}

	b.sendMarkdown(msg.Chat.ID, reply)
}

func startText() string {
	return `Welcome to the *SAME Telegram Bot*!

I'm your remote management companion for SAME (Stateless Agent Memory Engine). From right here in Telegram you can manage your vaults, search your knowledge base, chat with AI, and stay on top of your agent team.

Here's what I can do:

*Get started:*
/status -- check your vault health
/search <query> -- find anything in your vault
/ai on -- start an AI conversation
/help -- see all available commands

*Stay informed:*
I'll notify you about session completions, decisions that need your attention, and agent handoffs -- all configurable.

Type /help for the full command list, or just send me a message to get going.`
}

func (b *Bot) helpText() string {
	return b.generateHelpText()
}

func cmdStatus() (string, error) {
	out, err := exec.RunSame("status")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("*Vault Status*\n\n```\n%s\n```", out), nil
}

func cmdDoctor() (string, error) {
	out, err := exec.Doctor()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("*Doctor Report*\n\n```\n%s\n```", out), nil
}

const maxSearchResults = 5
const maxSnippetLen = 150

func cmdSearch(query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "Usage: /search <query>", nil
	}
	results, err := exec.SearchJSON(query)
	if err != nil {
		// Fallback: if JSON parsing fails, show raw output
		out, err2 := exec.Search(query)
		if err2 != nil {
			return "", err2
		}
		if strings.TrimSpace(out) == "" {
			return fmt.Sprintf("No results found for \"%s\".", escapeMarkdown(query)), nil
		}
		if len(out) > 3500 {
			out = out[:3500] + "\n... (truncated)"
		}
		return fmt.Sprintf("*Search: %s*\n\n```\n%s\n```", escapeMarkdown(query), out), nil
	}
	if len(results) == 0 {
		return fmt.Sprintf("No results found for \"%s\".", escapeMarkdown(query)), nil
	}
	return formatSearchResults(query, results), nil
}

func formatSearchResults(query string, results []exec.SearchResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("*Search: %s*\n", escapeMarkdown(query)))

	limit := maxSearchResults
	if len(results) < limit {
		limit = len(results)
	}

	for i := 0; i < limit; i++ {
		r := results[i]

		title := r.Title
		if title == "" {
			title = pathBaseName(r.Path)
		}

		snippet := strings.TrimSpace(r.Snippet)
		if len(snippet) > maxSnippetLen {
			snippet = snippet[:maxSnippetLen] + "..."
		}

		b.WriteString(fmt.Sprintf("\n*%d.* *%s*\n", i+1, escapeMarkdown(title)))
		if r.Path != "" {
			b.WriteString(fmt.Sprintf("`%s`\n", r.Path))
		}
		if snippet != "" {
			b.WriteString(fmt.Sprintf("_%s_\n", escapeMarkdown(snippet)))
		}

		// Score and type on one line
		meta := fmt.Sprintf("Score: %.2f", r.Score)
		if r.Type != "" {
			meta += fmt.Sprintf(" | Type: %s", r.Type)
		}
		b.WriteString(meta + "\n")
	}

	if len(results) > limit {
		b.WriteString(fmt.Sprintf("\n_%d more results not shown._", len(results)-limit))
	}

	return b.String()
}

// pathBaseName returns the last component of a path, without extension.
func pathBaseName(p string) string {
	// Find last slash
	i := strings.LastIndex(p, "/")
	if i >= 0 {
		p = p[i+1:]
	}
	// Strip extension
	if dot := strings.LastIndex(p, "."); dot > 0 {
		p = p[:dot]
	}
	return p
}

func cmdAsk(question string) (string, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return "Usage: /ask <question>", nil
	}
	out, err := exec.Ask(question)
	if err != nil {
		return "", err
	}
	if len(out) > 3500 {
		out = out[:3500] + "\n... (truncated)"
	}
	return fmt.Sprintf("*Answer*\n\n%s", escapeMarkdown(out)), nil
}

func cmdVaults() (string, error) {
	out, err := exec.RunSame("vault", "list")
	if err != nil {
		out, err = exec.RunSame("status")
		if err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("*Vaults*\n\n```\n%s\n```", out), nil
}

func cmdDigest() (string, error) {
	out, err := exec.RunSame("status")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("*On-Demand Digest*\n\n```\n%s\n```", out), nil
}

func cmdConfig(b *Bot) string {
	return fmt.Sprintf(`*Current Configuration*

*Notifications:*
- Session end: %v
- Decisions: %v
- Handoffs: %v

*Digest:*
- Enabled: %v
- Time: %s

*Allowed users:* %d configured`,
		b.cfg.Notify.SessionEnd,
		b.cfg.Notify.Decisions,
		b.cfg.Notify.Handoffs,
		b.cfg.Digest.Enabled,
		b.cfg.Digest.Time,
		len(b.cfg.Bot.AllowedUserIDs),
	)
}
