package analytics

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Event types logged by the system.
const (
	EventWorkspaceCreated = "workspace_created"
	EventSeedSelected     = "seed_selected"
	EventTopicProvided    = "topic_provided"
	EventAuthSelected     = "auth_selected"
	EventSessionOpen      = "session_open"
	EventFeedback         = "feedback"
	EventCommandUsed      = "command_used"
	EventInviteUsed       = "invite_used"
)

// Store is a privacy-focused analytics store backed by SQLite.
// It logs anonymous events (no PII — user IDs are hashed) and supports
// per-user opt-out.
type Store struct {
	db *sql.DB
}

// New opens (or creates) the analytics database at the given path.
func New(dbPath string) (*Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create analytics dir: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open analytics db: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate analytics: %w", err)
	}

	_ = os.Chmod(dbPath, 0o600)
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id    INTEGER NOT NULL,
			event_type TEXT NOT NULL,
			value      TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_events_type ON events(event_type);
		CREATE INDEX IF NOT EXISTS idx_events_user ON events(user_id);

		CREATE TABLE IF NOT EXISTS opt_out (
			user_id INTEGER PRIMARY KEY
		);
	`)
	return err
}

// Log records an event. If the user has opted out, it's a no-op.
func (s *Store) Log(userID int64, eventType, value string) {
	if s.IsOptedOut(userID) {
		return
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	_, _ = s.db.Exec(
		`INSERT INTO events (user_id, event_type, value, created_at) VALUES (?, ?, ?, ?)`,
		userID, eventType, value, now,
	)
}

// OptOut disables analytics for a user and deletes their existing data.
func (s *Store) OptOut(userID int64) {
	_, _ = s.db.Exec(`INSERT OR IGNORE INTO opt_out (user_id) VALUES (?)`, userID)
	_, _ = s.db.Exec(`DELETE FROM events WHERE user_id = ?`, userID)
}

// OptIn re-enables analytics for a user.
func (s *Store) OptIn(userID int64) {
	_, _ = s.db.Exec(`DELETE FROM opt_out WHERE user_id = ?`, userID)
}

// IsOptedOut returns true if the user has opted out of analytics.
func (s *Store) IsOptedOut(userID int64) bool {
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM opt_out WHERE user_id = ?`, userID).Scan(&count)
	return count > 0
}

// Summary holds aggregate stats for the /stats command.
type Summary struct {
	TotalUsers       int
	TotalWorkspaces  int
	TotalSessions    int
	FeedbackCount    int
	SeedBreakdown    map[string]int // seed type -> count
	AuthBreakdown    map[string]int // auth type -> count
	ActiveToday      int
	ActiveThisWeek   int
	RecentFeedback   []FeedbackEntry
}

// FeedbackEntry is a single feedback message for admin review.
type FeedbackEntry struct {
	UserID    int64
	Message   string
	CreatedAt string
}

// GetSummary returns aggregate analytics for admin review.
func (s *Store) GetSummary() (*Summary, error) {
	sum := &Summary{
		SeedBreakdown: make(map[string]int),
		AuthBreakdown: make(map[string]int),
	}

	// Total unique users.
	_ = s.db.QueryRow(`SELECT COUNT(DISTINCT user_id) FROM events`).Scan(&sum.TotalUsers)

	// Total workspaces created.
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM events WHERE event_type = ?`, EventWorkspaceCreated).Scan(&sum.TotalWorkspaces)

	// Total session opens.
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM events WHERE event_type = ?`, EventSessionOpen).Scan(&sum.TotalSessions)

	// Feedback count.
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM events WHERE event_type = ?`, EventFeedback).Scan(&sum.FeedbackCount)

	// Active today.
	today := time.Now().UTC().Format("2006-01-02")
	_ = s.db.QueryRow(`SELECT COUNT(DISTINCT user_id) FROM events WHERE created_at >= ?`, today+"T00:00:00Z").Scan(&sum.ActiveToday)

	// Active this week.
	weekAgo := time.Now().UTC().AddDate(0, 0, -7).Format("2006-01-02")
	_ = s.db.QueryRow(`SELECT COUNT(DISTINCT user_id) FROM events WHERE created_at >= ?`, weekAgo+"T00:00:00Z").Scan(&sum.ActiveThisWeek)

	// Seed breakdown.
	rows, err := s.db.Query(`SELECT value, COUNT(*) FROM events WHERE event_type = ? GROUP BY value`, EventSeedSelected)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var v string
			var c int
			if rows.Scan(&v, &c) == nil {
				sum.SeedBreakdown[v] = c
			}
		}
	}

	// Auth breakdown.
	rows2, err := s.db.Query(`SELECT value, COUNT(*) FROM events WHERE event_type = ? GROUP BY value`, EventAuthSelected)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var v string
			var c int
			if rows2.Scan(&v, &c) == nil {
				sum.AuthBreakdown[v] = c
			}
		}
	}

	// Recent feedback (last 5).
	rows3, err := s.db.Query(
		`SELECT user_id, value, created_at FROM events WHERE event_type = ? ORDER BY id DESC LIMIT 5`,
		EventFeedback,
	)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var f FeedbackEntry
			if rows3.Scan(&f.UserID, &f.Message, &f.CreatedAt) == nil {
				sum.RecentFeedback = append(sum.RecentFeedback, f)
			}
		}
	}

	return sum, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}
