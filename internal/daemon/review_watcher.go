package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sgx-labs/same-telegram/internal/bot"
)

// ReviewCategory describes which company-hq subdirectory a file came from.
type ReviewCategory string

const (
	CategoryReview   ReviewCategory = "review"
	CategoryDecision ReviewCategory = "decision"
	CategoryReport   ReviewCategory = "report"
	CategoryReply    ReviewCategory = "reply"
)

// watchedDir pairs a directory path with its category.
type watchedDir struct {
	Path     string
	Category ReviewCategory
}

// ReviewWatcher watches company-hq directories for new files and notifies
// the CEO via Telegram.
type ReviewWatcher struct {
	bot    *bot.Bot
	logger *log.Logger
	dirs   []watchedDir

	// seen tracks files we've already processed (keyed by absolute path).
	mu   sync.Mutex
	seen map[string]bool
}

// NewReviewWatcher creates a watcher for review, decision, and reply directories.
// baseDir is the company-hq root (from SAME_COMPANY_HQ env or config).
// extraDirs allows adding custom watched directories via config.
func NewReviewWatcher(b *bot.Bot, logger *log.Logger, baseDir string, extraDirs map[string]ReviewCategory) *ReviewWatcher {
	dirs := []watchedDir{
		{Path: filepath.Join(baseDir, "reviews", "pending"), Category: CategoryReview},
		{Path: filepath.Join(baseDir, "decisions"), Category: CategoryDecision},
		{Path: filepath.Join(baseDir, "messages", "inbound"), Category: CategoryReply},
	}

	for path, cat := range extraDirs {
		dirs = append(dirs, watchedDir{Path: path, Category: cat})
	}

	return &ReviewWatcher{
		bot:    b,
		logger: logger,
		dirs:   dirs,
		seen:   make(map[string]bool),
	}
}

// Watch starts watching directories for new files. Blocks until ctx is cancelled.
func (rw *ReviewWatcher) Watch(ctx context.Context) {
	// Ensure all watched directories exist
	for _, wd := range rw.dirs {
		if err := os.MkdirAll(wd.Path, 0o755); err != nil {
			rw.logger.Printf("review-watcher: cannot create %s: %v", wd.Path, err)
		}
	}

	// Mark existing files as seen so we don't notify on startup
	rw.seedExisting()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		rw.logger.Printf("review-watcher: failed to create fsnotify watcher: %v", err)
		rw.logger.Println("review-watcher: falling back to polling mode")
		rw.pollLoop(ctx)
		return
	}
	defer watcher.Close()

	for _, wd := range rw.dirs {
		if err := watcher.Add(wd.Path); err != nil {
			rw.logger.Printf("review-watcher: cannot watch %s: %v (skipping)", wd.Path, err)
		} else {
			rw.logger.Printf("review-watcher: watching %s [%s]", wd.Path, wd.Category)
		}
	}

	// Debounce timer to batch rapid writes (editors often write temp then rename)
	debounce := make(map[string]ReviewCategory)
	var debounceMu sync.Mutex
	timer := time.NewTimer(500 * time.Millisecond)
	timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
				continue
			}

			cat := rw.categoryFor(event.Name)
			if cat == "" {
				continue
			}

			debounceMu.Lock()
			debounce[event.Name] = cat
			debounceMu.Unlock()
			timer.Reset(500 * time.Millisecond)

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			rw.logger.Printf("review-watcher: error: %v", err)

		case <-timer.C:
			debounceMu.Lock()
			batch := debounce
			debounce = make(map[string]ReviewCategory)
			debounceMu.Unlock()

			for path, cat := range batch {
				rw.processFile(path, cat)
			}
		}
	}
}

// pollLoop is a fallback if fsnotify is unavailable.
func (rw *ReviewWatcher) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rw.scanAll()
		}
	}
}

// scanAll checks all watched directories for new files.
func (rw *ReviewWatcher) scanAll() {
	for _, wd := range rw.dirs {
		entries, err := os.ReadDir(wd.Path)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(wd.Path, e.Name())
			rw.processFile(path, wd.Category)
		}
	}
}

// seedExisting marks all current files as already seen.
func (rw *ReviewWatcher) seedExisting() {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	for _, wd := range rw.dirs {
		entries, err := os.ReadDir(wd.Path)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				rw.seen[filepath.Join(wd.Path, e.Name())] = true
			}
		}
	}
}

// processFile reads a new file and sends a notification.
func (rw *ReviewWatcher) processFile(path string, category ReviewCategory) {
	rw.mu.Lock()
	if rw.seen[path] {
		rw.mu.Unlock()
		return
	}
	rw.seen[path] = true
	rw.mu.Unlock()

	// Skip directories and temp files
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return
	}
	name := filepath.Base(path)
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".tmp") || strings.HasSuffix(name, ".swp") || strings.HasSuffix(name, "~") {
		return
	}

	summary := extractSummary(path)
	rw.logger.Printf("review-watcher: new %s file: %s", category, name)

	rw.bot.SendReviewNotification(category.String(), name, summary, category == CategoryDecision, category == CategoryReply)
}

// categoryFor returns the category for a file path, or "" if not in a watched dir.
func (rw *ReviewWatcher) categoryFor(path string) ReviewCategory {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	for _, wd := range rw.dirs {
		wdAbs, err := filepath.Abs(wd.Path)
		if err != nil {
			wdAbs = wd.Path
		}
		if strings.HasPrefix(abs, wdAbs+string(os.PathSeparator)) || abs == wdAbs {
			return wd.Category
		}
	}
	return ""
}

// extractSummary reads the first ~500 chars of a file to produce a summary.
// It looks for common header patterns (Agent:, Summary:, Title:, Subject:).
func extractSummary(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "(could not read file)"
	}

	content := string(data)
	if len(content) > 500 {
		content = content[:500]
	}

	// Try to extract structured header fields
	var parts []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "agent:") ||
			strings.HasPrefix(lower, "summary:") ||
			strings.HasPrefix(lower, "title:") ||
			strings.HasPrefix(lower, "subject:") ||
			strings.HasPrefix(lower, "status:") ||
			strings.HasPrefix(lower, "priority:") {
			parts = append(parts, line)
		}
	}

	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}

	// No structured headers -- return first few lines
	lines := strings.SplitN(content, "\n", 6)
	if len(lines) > 5 {
		lines = lines[:5]
	}
	result := strings.TrimSpace(strings.Join(lines, "\n"))
	if result == "" {
		return "(empty file)"
	}
	return result
}

// String returns the display name for a ReviewCategory.
func (c ReviewCategory) String() string {
	switch c {
	case CategoryReview:
		return "Review"
	case CategoryDecision:
		return "Decision"
	case CategoryReport:
		return "Report"
	case CategoryReply:
		return "Reply"
	default:
		return string(c)
	}
}

// CompanyHQDir returns the base path to company-hq.
// Uses SAME_COMPANY_HQ env var if set, otherwise defaults to ~/Projects/same-company/company-hq.
func CompanyHQDir() string {
	if dir := os.Getenv("SAME_COMPANY_HQ"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Projects", "same-company", "company-hq")
}

// FormatReviewNotification builds a Telegram-ready notification string.
func FormatReviewNotification(category, filename, summary string, isDecision, isReply bool) string {
	emoji := "📄"
	switch strings.ToLower(category) {
	case "review":
		emoji = "📋"
	case "decision":
		emoji = "⚖️"
	case "report":
		emoji = "📊"
	case "reply":
		emoji = "💬"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s *New %s*\n\n", emoji, category))
	b.WriteString(fmt.Sprintf("*File:* `%s`\n\n", filename))

	// Preview: first 200 chars of summary
	preview := summary
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	b.WriteString(preview)

	// Add command instructions
	b.WriteString("\n\n")
	switch {
	case isDecision:
		b.WriteString("Use /decisions to view")
	case isReply:
		b.WriteString("Use /messages to view replies")
	default:
		name := strings.TrimSuffix(filename, filepath.Ext(filename))
		b.WriteString(fmt.Sprintf("Use /review %s to read", name))
	}

	return b.String()
}
