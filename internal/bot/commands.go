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

	switch cmd {
	case "start", "help":
		reply = helpText()
	case "status":
		reply, err = cmdStatus()
	case "doctor":
		reply, err = cmdDoctor()
	case "search":
		reply, err = cmdSearch(args)
	case "ask":
		reply, err = cmdAsk(args)
	case "vaults":
		reply, err = cmdVaults()
	case "digest":
		reply, err = cmdDigest()
	case "config":
		reply = cmdConfig(b)
	default:
		reply = fmt.Sprintf("Unknown command: /%s\nUse /help for available commands.", cmd)
	}

	if err != nil {
		reply = fmt.Sprintf("❌ Error: %s", err)
	}

	b.sendMarkdown(msg.Chat.ID, reply)
}

func helpText() string {
	return `🤖 *SAME Telegram Bot*

*Management Commands:*
/status — vault status
/doctor — run health check
/search <query> — semantic search vault
/ask <question> — ask SAME a question
/vaults — list/switch vaults
/digest — on-demand daily digest
/config — view current settings
/help — this message`
}

func cmdStatus() (string, error) {
	out, err := exec.RunSame("status")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("📊 *Vault Status*\n\n```\n%s\n```", out), nil
}

func cmdDoctor() (string, error) {
	out, err := exec.Doctor()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("🔍 *Doctor Report*\n\n```\n%s\n```", out), nil
}

func cmdSearch(query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "Usage: /search <query>", nil
	}
	out, err := exec.Search(query)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) == "" {
		return fmt.Sprintf("No results for: %s", escapeMarkdown(query)), nil
	}
	// Truncate long results for Telegram's 4096 char limit
	if len(out) > 3500 {
		out = out[:3500] + "\n... (truncated)"
	}
	return fmt.Sprintf("🔎 *Search: %s*\n\n```\n%s\n```", escapeMarkdown(query), out), nil
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
	return fmt.Sprintf("💡 *Answer*\n\n%s", escapeMarkdown(out)), nil
}

func cmdVaults() (string, error) {
	out, err := exec.RunSame("vault", "list")
	if err != nil {
		// Fallback: try without subcommand
		out, err = exec.RunSame("status")
		if err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("🗄 *Vaults*\n\n```\n%s\n```", out), nil
}

func cmdDigest() (string, error) {
	out, err := exec.RunSame("status")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("📊 *On-Demand Digest*\n\n```\n%s\n```", out), nil
}

func cmdConfig(b *Bot) string {
	return fmt.Sprintf(`⚙️ *Current Configuration*

*Notifications:*
• Session end: %v
• Decisions: %v
• Handoffs: %v

*Digest:*
• Enabled: %v
• Time: %s

*Allowed users:* %d configured`,
		b.cfg.Notify.SessionEnd,
		b.cfg.Notify.Decisions,
		b.cfg.Notify.Handoffs,
		b.cfg.Digest.Enabled,
		b.cfg.Digest.Time,
		len(b.cfg.Bot.AllowedUserIDs),
	)
}
