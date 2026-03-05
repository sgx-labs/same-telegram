package bot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// teamConfig represents the team config.json structure.
type teamConfig struct {
	Name    string       `json:"name"`
	Members []teamMember `json:"members"`
}

// teamMember represents an agent in the team config.
type teamMember struct {
	Name      string `json:"name"`
	AgentType string `json:"agentType"`
	Model     string `json:"model"`
}

// teamConfigPath returns the path to the team config.
func teamConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "teams", "same-company", "config.json")
}

// readTeamConfig reads and parses the team config.json.
func readTeamConfig() (*teamConfig, error) {
	data, err := os.ReadFile(teamConfigPath())
	if err != nil {
		return nil, err
	}
	var cfg teamConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse team config: %w", err)
	}
	return &cfg, nil
}

// companyHQDir returns the path to the company-hq directory.
func companyHQDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Projects", "same-company", "company-hq")
}

// cmdTeam returns agent team status by reading the team config and company-hq.
func cmdTeam() (string, error) {
	var b strings.Builder
	b.WriteString("👥 *Agent Team Status*\n\n")

	// Read team members from config
	cfg, err := readTeamConfig()
	if err == nil && len(cfg.Members) > 0 {
		b.WriteString(fmt.Sprintf("*Team:* %s\n", escapeMarkdown(cfg.Name)))
		b.WriteString(fmt.Sprintf("*Members:* %d\n\n", len(cfg.Members)))
		for _, m := range cfg.Members {
			role := m.AgentType
			if role == "" {
				role = "agent"
			}
			b.WriteString(fmt.Sprintf("• *%s* — %s (%s)\n",
				escapeMarkdown(m.Name),
				escapeMarkdown(role),
				escapeMarkdown(m.Model)))
		}
		b.WriteString("\n")
	} else {
		b.WriteString("_Team config not available_\n\n")
	}

	// Decision/announcement counts
	hq := companyHQDir()
	pending, _ := countFiles(filepath.Join(hq, "decisions", "pending"))
	approved, _ := countFiles(filepath.Join(hq, "decisions", "approved"))
	rejected, _ := countFiles(filepath.Join(hq, "decisions", "rejected"))
	announcements, _ := countFiles(filepath.Join(hq, "announcements"))

	b.WriteString("*Decisions:*\n")
	b.WriteString(fmt.Sprintf("• Pending: %d\n", pending))
	b.WriteString(fmt.Sprintf("• Approved: %d\n", approved))
	b.WriteString(fmt.Sprintf("• Rejected: %d\n", rejected))
	b.WriteString(fmt.Sprintf("\n*Announcements:* %d posted\n", announcements))

	return b.String(), nil
}

// cmdDecisions lists pending decisions with approve/reject buttons.
func cmdDecisions() (string, []pendingDecision, error) {
	dir := filepath.Join(companyHQDir(), "decisions", "pending")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "No pending decisions directory found.", nil, nil
		}
		return "", nil, fmt.Errorf("read decisions: %w", err)
	}

	var decisions []pendingDecision
	for _, e := range entries {
		if e.IsDir() || e.Name() == ".gitkeep" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		decisions = append(decisions, pendingDecision{
			Filename: e.Name(),
			Content:  string(content),
		})
	}

	if len(decisions) == 0 {
		return "📋 No pending decisions.", nil, nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("📋 *Pending Decisions (%d)*\n", len(decisions)))
	for i, d := range decisions {
		title := decisionTitle(d.Content, d.Filename)
		b.WriteString(fmt.Sprintf("\n*%d.* %s\n", i+1, escapeMarkdown(title)))
		preview := decisionPreview(d.Content)
		if preview != "" {
			b.WriteString(escapeMarkdown(preview))
			b.WriteString("\n")
		}
	}

	return b.String(), decisions, nil
}

// pendingDecision holds a decision file's name and content.
type pendingDecision struct {
	Filename string
	Content  string
}

// DecisionKeyboard creates an inline keyboard with approve/reject per decision.
func DecisionKeyboard(filename string) tgbotapi.InlineKeyboardMarkup {
	id := strings.TrimSuffix(filename, filepath.Ext(filename))
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Approve", "approve:"+id),
			tgbotapi.NewInlineKeyboardButtonData("❌ Reject", "reject:"+id),
		),
	)
}

// handleDecisionAction moves a decision file to the approved/ or rejected/ directory.
func (b *Bot) handleDecisionAction(chatID int64, decisionID, action string) {
	hq := companyHQDir()
	pendingDir := filepath.Join(hq, "decisions", "pending")
	targetDir := filepath.Join(hq, "decisions", action)

	// Find the matching file (decisionID is filename without extension)
	entries, err := os.ReadDir(pendingDir)
	if err != nil {
		b.sendMarkdown(chatID, fmt.Sprintf("Failed to read pending decisions: %s", err))
		return
	}

	var srcPath string
	var srcName string
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if name == decisionID {
			srcPath = filepath.Join(pendingDir, e.Name())
			srcName = e.Name()
			break
		}
	}

	if srcPath == "" {
		b.sendMarkdown(chatID, fmt.Sprintf("Decision `%s` not found in pending.", escapeMarkdown(decisionID)))
		return
	}

	// Append CEO decision stamp to the file
	stamp := fmt.Sprintf("\n\n---\n**CEO Decision:** %s at %s\n",
		strings.ToUpper(action), time.Now().Format(time.RFC3339))
	f, err := os.OpenFile(srcPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		f.WriteString(stamp)
		f.Close()
	}

	// Move to target directory
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		b.sendMarkdown(chatID, fmt.Sprintf("Failed to create %s directory: %s", action, err))
		return
	}

	dstPath := filepath.Join(targetDir, srcName)
	if err := os.Rename(srcPath, dstPath); err != nil {
		b.sendMarkdown(chatID, fmt.Sprintf("Failed to move decision: %s", err))
		return
	}

	emoji := "✅"
	if action == "rejected" {
		emoji = "❌"
	}
	b.sendMarkdown(chatID, fmt.Sprintf("%s Decision *%s* %s.", emoji, escapeMarkdown(decisionID), action))
}

// cmdAnnounce writes a CEO announcement to company-hq/announcements/.
func cmdAnnounce(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "Usage: /announce <message>", nil
	}

	dir := filepath.Join(companyHQDir(), "announcements")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create announcements dir: %w", err)
	}

	timestamp := time.Now().Format("2006-01-02-150405")
	filename := fmt.Sprintf("%s.md", timestamp)
	path := filepath.Join(dir, filename)

	content := fmt.Sprintf("# CEO Announcement\n\n%s\n\n---\n*Posted: %s*\n",
		text, time.Now().Format(time.RFC3339))

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write announcement: %w", err)
	}

	return fmt.Sprintf("📢 *Announcement Posted*\n\nSaved to `%s`", escapeMarkdown(filename)), nil
}

// decisionTitle extracts the first heading or filename as the title.
func decisionTitle(content, filename string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return strings.TrimSuffix(filename, filepath.Ext(filename))
}

// decisionPreview returns the first non-heading, non-empty line as a preview.
func decisionPreview(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "---") {
			continue
		}
		if len(line) > 120 {
			return line[:120] + "..."
		}
		return line
	}
	return ""
}

// countFiles counts non-hidden, non-gitkeep files in a directory.
func countFiles(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && e.Name() != ".gitkeep" && !strings.HasPrefix(e.Name(), ".") {
			count++
		}
	}
	return count, nil
}
