package bot

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sgx-labs/same-telegram/internal/audit"
	"github.com/sgx-labs/same-telegram/internal/msgbox"
)

// reviewFile holds metadata about a pending review file.
type reviewFile struct {
	Name     string
	Title    string // first line of file
	Size     int64
	FullPath string
}

// pendingReviewsDir returns the path to reviews/pending/.
func pendingReviewsDir() string {
	return filepath.Join(msgbox.CompanyHQDir(), "reviews", "pending")
}

// listPendingReviews reads all files in reviews/pending/ and returns metadata.
func listPendingReviews() ([]reviewFile, error) {
	dir := pendingReviewsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read reviews dir: %w", err)
	}

	var files []reviewFile
	for _, e := range entries {
		if e.IsDir() || e.Name() == ".gitkeep" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fullPath := filepath.Join(dir, e.Name())
		title := extractFirstLine(fullPath)
		files = append(files, reviewFile{
			Name:     e.Name(),
			Title:    title,
			Size:     info.Size(),
			FullPath: fullPath,
		})
	}
	return files, nil
}

// extractFirstLine reads the first non-empty line of a file, stripping markdown heading prefix.
func extractFirstLine(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "(unreadable)"
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "---" {
			continue
		}
		// Strip markdown heading prefix
		line = strings.TrimLeft(line, "# ")
		if len(line) > 80 {
			return line[:80] + "..."
		}
		return line
	}
	return "(empty)"
}

// resolveReviewFile finds a review file by number (1-based) or partial name match.
func resolveReviewFile(arg string, files []reviewFile) (*reviewFile, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return nil, fmt.Errorf("specify a file number or name")
	}

	// Try as number first
	if num, err := strconv.Atoi(arg); err == nil {
		if num < 1 || num > len(files) {
			return nil, fmt.Errorf("number %d out of range (1-%d)", num, len(files))
		}
		return &files[num-1], nil
	}

	// Try exact filename match
	for i, f := range files {
		if f.Name == arg {
			return &files[i], nil
		}
	}

	// Try partial match (case-insensitive)
	lower := strings.ToLower(arg)
	for i, f := range files {
		if strings.Contains(strings.ToLower(f.Name), lower) {
			return &files[i], nil
		}
	}

	return nil, fmt.Errorf("no review file matching %q", arg)
}

// cmdReviews lists all pending review files.
func cmdReviews() (string, error) {
	files, err := listPendingReviews()
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "No pending reviews.", nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("*Pending Reviews (%d)*\n\n", len(files)))
	for i, f := range files {
		b.WriteString(fmt.Sprintf("*%d.* `%s`\n", i+1, f.Name))
		b.WriteString(fmt.Sprintf("    %s (%s)\n\n", escapeMarkdown(f.Title), formatSize(f.Size)))
	}
	b.WriteString("Use /review \\_number\\_ to read a file.")
	return b.String(), nil
}

// cmdReview shows the contents of a specific review file.
// Returns a slice of messages (split if content exceeds Telegram limit).
func cmdReview(arg string) ([]string, error) {
	files, err := listPendingReviews()
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return []string{"No pending reviews."}, nil
	}

	f, err := resolveReviewFile(arg, files)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(f.FullPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	content := string(data)
	header := fmt.Sprintf("*Review:* `%s`\n\n", f.Name)

	// If content fits in one message, send it directly
	const maxLen = 3500 // leave room for header and formatting
	if len(content) <= maxLen {
		return []string{header + escapeMarkdown(content)}, nil
	}

	// Split at natural boundaries
	chunks := splitContent(content, maxLen)
	var messages []string
	for i, chunk := range chunks {
		prefix := header
		if i > 0 {
			prefix = fmt.Sprintf("*Review:* `%s` (part %d/%d)\n\n", f.Name, i+1, len(chunks))
		} else if len(chunks) > 1 {
			prefix = fmt.Sprintf("*Review:* `%s` (part 1/%d)\n\n", f.Name, len(chunks))
		}
		messages = append(messages, prefix+escapeMarkdown(chunk))
	}
	return messages, nil
}

// cmdApproveReview moves a review file to reviews/approved/.
func cmdApproveReview(arg string) (string, error) {
	return moveReview(arg, "approved")
}

// cmdRejectReview moves a review file to reviews/rejected/.
func cmdRejectReview(arg string) (string, error) {
	return moveReview(arg, "rejected")
}

// moveReview moves a pending review file to the specified target subdirectory.
// It writes an audit log entry BEFORE performing the move for crash atomicity.
func moveReview(arg, action string) (string, error) {
	files, err := listPendingReviews()
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "No pending reviews.", nil
	}

	f, err := resolveReviewFile(arg, files)
	if err != nil {
		return "", err
	}

	targetDir := filepath.Join(msgbox.CompanyHQDir(), "reviews", action)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("create %s dir: %w", action, err)
	}

	// Write audit log BEFORE performing any mutation (crash atomicity)
	audit.LogReviewAction(action, f.Name, "intent")

	// Append CEO action stamp
	stamp := fmt.Sprintf("\n\n---\n**CEO Decision:** %s at %s\n",
		strings.ToUpper(action), time.Now().Format(time.RFC3339))
	appendF, err := os.OpenFile(f.FullPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		appendF.WriteString(stamp)
		appendF.Close()
	}

	dst := filepath.Join(targetDir, f.Name)
	if err := os.Rename(f.FullPath, dst); err != nil {
		audit.LogReviewAction(action, f.Name, "rename_failed: "+err.Error())
		return "", fmt.Errorf("move file: %w", err)
	}

	audit.LogReviewAction(action, f.Name, "ok")

	label := "Approved"
	if action == "rejected" {
		label = "Rejected"
	}
	return fmt.Sprintf("*%s* `%s`.", label, f.Name), nil
}

// cmdDecisionsFile shows the contents of company-hq/decisions.md.
func cmdDecisionsFile() ([]string, error) {
	path := filepath.Join(msgbox.CompanyHQDir(), "decisions.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{"No decisions.md file found."}, nil
		}
		return nil, fmt.Errorf("read decisions.md: %w", err)
	}

	content := string(data)
	header := "*Decisions*\n\n"

	const maxLen = 3500
	if len(content) <= maxLen {
		return []string{header + escapeMarkdown(content)}, nil
	}

	chunks := splitContent(content, maxLen)
	var messages []string
	for i, chunk := range chunks {
		prefix := header
		if i > 0 {
			prefix = fmt.Sprintf("*Decisions* (part %d/%d)\n\n", i+1, len(chunks))
		} else if len(chunks) > 1 {
			prefix = fmt.Sprintf("*Decisions* (part 1/%d)\n\n", len(chunks))
		}
		messages = append(messages, prefix+escapeMarkdown(chunk))
	}
	return messages, nil
}

// splitContent splits text at natural boundaries (## headings or every maxLen chars).
func splitContent(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var chunks []string
	remaining := text

	for len(remaining) > 0 {
		if len(remaining) <= maxLen {
			chunks = append(chunks, remaining)
			break
		}

		// Look for a ## heading boundary within the limit
		chunk := remaining[:maxLen]
		splitAt := -1

		// Search backwards for a ## heading
		lines := strings.Split(chunk, "\n")
		pos := 0
		lastHeading := -1
		for _, line := range lines {
			lineLen := len(line) + 1 // +1 for newline
			if strings.HasPrefix(strings.TrimSpace(line), "## ") && pos > 500 {
				lastHeading = pos
			}
			pos += lineLen
		}

		if lastHeading > 0 {
			splitAt = lastHeading
		}

		// Fall back to last double-newline
		if splitAt < 0 {
			if idx := strings.LastIndex(chunk, "\n\n"); idx > 500 {
				splitAt = idx + 1
			}
		}

		// Fall back to last single newline
		if splitAt < 0 {
			if idx := strings.LastIndex(chunk, "\n"); idx > 500 {
				splitAt = idx + 1
			}
		}

		// Last resort: hard split
		if splitAt < 0 {
			splitAt = maxLen
		}

		chunks = append(chunks, remaining[:splitAt])
		remaining = remaining[splitAt:]
	}

	return chunks
}

// formatSize returns a human-readable file size string.
func formatSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
