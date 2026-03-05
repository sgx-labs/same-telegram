package bot

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/msgbox"
)

// agentRolePattern matches review filenames like "2026-03-05-backend-dev-something.md"
// to extract the agent role portion.
var agentRolePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-(.+?)-[^-]+\.md$`)

// cmdTeam returns agent team status by reading real state from company-hq.
func cmdTeam() (string, error) {
	hq := msgbox.CompanyHQDir()
	var b strings.Builder
	b.WriteString("*Agent Team Status*\n\n")

	// --- Reviews ---
	reviewsPending, _ := countFiles(filepath.Join(hq, "reviews", "pending"))
	reviewsApproved, _ := countFiles(filepath.Join(hq, "reviews", "approved"))
	reviewsRejected, _ := countFiles(filepath.Join(hq, "reviews", "rejected"))
	totalReviews := reviewsPending + reviewsApproved + reviewsRejected

	b.WriteString("*Reviews:*\n")
	b.WriteString(fmt.Sprintf("  Pending: %d\n", reviewsPending))
	b.WriteString(fmt.Sprintf("  Approved: %d\n", reviewsApproved))
	b.WriteString(fmt.Sprintf("  Rejected: %d\n", reviewsRejected))
	b.WriteString(fmt.Sprintf("  Total: %d\n\n", totalReviews))

	// --- Decisions ---
	decisionCount, _ := countFilesRecursive(filepath.Join(hq, "decisions"))
	b.WriteString(fmt.Sprintf("*Decisions:* %d\n\n", decisionCount))

	// --- Tasks ---
	taskDirs := []string{"queued", "in-progress", "blocked", "review", "done", "failed"}
	totalTasks := 0
	var taskLines []string
	for _, state := range taskDirs {
		n, _ := countFiles(filepath.Join(hq, "tasks", state))
		if n > 0 {
			taskLines = append(taskLines, fmt.Sprintf("  %s: %d", state, n))
			totalTasks += n
		}
	}
	b.WriteString(fmt.Sprintf("*Tasks:* %d\n", totalTasks))
	for _, line := range taskLines {
		b.WriteString(line + "\n")
	}
	if totalTasks == 0 {
		b.WriteString("  (none)\n")
	}
	b.WriteString("\n")

	// --- Active agents (parsed from review filenames) ---
	agents := discoverActiveAgents(hq)
	if len(agents) > 0 {
		b.WriteString(fmt.Sprintf("*Active Agents:* %d\n", len(agents)))
		for _, a := range agents {
			b.WriteString(fmt.Sprintf("  %s\n", a))
		}
		b.WriteString("\n")
	}

	// --- Last activity ---
	lastMod := findLastActivity(hq)
	if !lastMod.IsZero() {
		ago := time.Since(lastMod).Round(time.Minute)
		b.WriteString(fmt.Sprintf("*Last activity:* %s (%s ago)\n", lastMod.Format("2006-01-02 15:04"), formatDuration(ago)))
	} else {
		b.WriteString("*Last activity:* unknown\n")
	}

	return b.String(), nil
}

// discoverActiveAgents scans review filenames across pending/approved/rejected
// to find which agent roles have submitted reviews.
func discoverActiveAgents(hq string) []string {
	seen := make(map[string]bool)
	for _, sub := range []string{"pending", "approved", "rejected"} {
		dir := filepath.Join(hq, "reviews", sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			if m := agentRolePattern.FindStringSubmatch(e.Name()); m != nil {
				seen[m[1]] = true
			}
		}
	}
	var roles []string
	for r := range seen {
		roles = append(roles, r)
	}
	sort.Strings(roles)
	return roles
}

// findLastActivity returns the most recent modification time across
// reviews, decisions, and tasks directories.
func findLastActivity(hq string) time.Time {
	var latest time.Time
	dirs := []string{
		filepath.Join(hq, "reviews", "pending"),
		filepath.Join(hq, "reviews", "approved"),
		filepath.Join(hq, "reviews", "rejected"),
		filepath.Join(hq, "decisions"),
		filepath.Join(hq, "tasks"),
		filepath.Join(hq, "announcements"),
	}
	for _, dir := range dirs {
		walkDir(dir, func(info os.FileInfo) {
			if info.ModTime().After(latest) {
				latest = info.ModTime()
			}
		})
	}
	return latest
}

// walkDir calls fn for every non-hidden file found recursively under dir.
func walkDir(dir string, fn func(os.FileInfo)) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(dir, e.Name())
		if e.IsDir() {
			walkDir(full, fn)
			continue
		}
		info, err := e.Info()
		if err == nil {
			fn(info)
		}
	}
}

// countFilesRecursive counts non-hidden files recursively under dir.
func countFilesRecursive(dir string) (int, error) {
	count := 0
	walkDir(dir, func(_ os.FileInfo) {
		count++
	})
	return count, nil
}

// formatDuration returns a human-readable duration string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h >= 24 {
		days := h / 24
		h = h % 24
		if h == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd %dh", days, h)
	}
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

// cmdDecisions lists pending decisions with approve/reject buttons.
func cmdDecisions() (string, []pendingDecision, error) {
	dir := filepath.Join(msgbox.CompanyHQDir(), "decisions", "pending")
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
		return "No pending decisions.", nil, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Pending Decisions (%d)*\n", len(decisions)))
	for i, d := range decisions {
		title := decisionTitle(d.Content, d.Filename)
		sb.WriteString(fmt.Sprintf("\n*%d.* %s\n", i+1, escapeMarkdown(title)))
		preview := decisionPreview(d.Content)
		if preview != "" {
			sb.WriteString(escapeMarkdown(preview))
			sb.WriteString("\n")
		}
	}

	return sb.String(), decisions, nil
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
			tgbotapi.NewInlineKeyboardButtonData("Approve", "approve:"+id),
			tgbotapi.NewInlineKeyboardButtonData("Reject", "reject:"+id),
		),
	)
}

// handleDecisionAction moves a decision file to the approved/ or rejected/ directory.
func (b *Bot) handleDecisionAction(chatID int64, decisionID, action string) {
	hq := msgbox.CompanyHQDir()
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

	label := "Approved"
	if action == "rejected" {
		label = "Rejected"
	}
	b.sendMarkdown(chatID, fmt.Sprintf("*%s* Decision `%s`.", label, escapeMarkdown(decisionID)))
}

// cmdAnnounce writes a CEO announcement to company-hq/announcements/.
func cmdAnnounce(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "Usage: /announce <message>", nil
	}

	dir := filepath.Join(msgbox.CompanyHQDir(), "announcements")
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

	return fmt.Sprintf("*Announcement Posted*\n\nSaved to `%s`", escapeMarkdown(filename)), nil
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
