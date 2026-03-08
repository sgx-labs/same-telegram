package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry represents a single audit log record for a Claude invocation.
type Entry struct {
	Timestamp            time.Time `json:"timestamp"`
	UserID               int64     `json:"user_id"`
	Prompt               string    `json:"prompt"`
	Response             string    `json:"response"`
	SessionID            string    `json:"session_id"`
	DangerousPermissions bool      `json:"dangerous_permissions"`
}

// Logger writes audit entries as JSON lines to a file.
type Logger struct {
	mu   sync.Mutex
	path string
}

// logPath returns ~/.same/telegram-audit.log
func logPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".same", "telegram-audit.log")
}

// NewLogger creates a new audit logger that writes to ~/.same/telegram-audit.log.
func NewLogger() *Logger {
	return &Logger{path: logPath()}
}

// truncate returns at most maxLen characters from s.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// reviewAuditPath returns the path to the review audit log.
func reviewAuditPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".same", "data", "reviews")
	// Allow override via environment variable
	if hq := os.Getenv("SAME_COMPANY_HQ"); hq != "" {
		dir = filepath.Join(hq, "reviews")
	}
	return filepath.Join(dir, "audit.log")
}

// LogReviewAction writes an append-only audit line for review approve/reject actions.
// Format: YYYY-MM-DD HH:MM:SS | ACTION | filename | result
// Errors are silently ignored to avoid disrupting bot operation.
func LogReviewAction(action, filename, result string) {
	path := reviewAuditPath()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	line := fmt.Sprintf("%s | %s | %s | %s\n",
		time.Now().UTC().Format("2006-01-02 15:04:05"),
		action, filename, result)
	f.WriteString(line)
}

// Log records an audit entry. Errors are silently ignored to avoid
// disrupting bot operation.
func (l *Logger) Log(entry Entry) {
	// Truncate prompt and response to 200 chars
	entry.Prompt = truncate(entry.Prompt, 200)
	entry.Response = truncate(entry.Response, 200)

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(l.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()

	fmt.Fprint(f, string(data))
}
