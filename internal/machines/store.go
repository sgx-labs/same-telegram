package machines

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const timeFormat = "2006-01-02T15:04:05Z"

// SQLiteStore is a SQLite-backed implementation of MachineStore.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a MachineStore at the given path.
// It creates the user_machines table if it doesn't exist.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// Ensure parent directory exists.
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	s := &SQLiteStore{db: db}

	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	// Restrict database file permissions to owner-only.
	_ = os.Chmod(dbPath, 0o600)

	return s, nil
}

func (s *SQLiteStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS user_machines (
			user_id    TEXT PRIMARY KEY,
			machine_id TEXT NOT NULL DEFAULT '',
			volume_id  TEXT NOT NULL DEFAULT '',
			region     TEXT NOT NULL DEFAULT '',
			state      TEXT NOT NULL DEFAULT '',
			token      TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			last_used  TEXT NOT NULL DEFAULT ''
		)
	`)
	return err
}

// GetUserMachine retrieves a user's machine record. Returns nil, nil if not found.
func (s *SQLiteStore) GetUserMachine(userID string) (*UserMachine, error) {
	row := s.db.QueryRow(
		`SELECT user_id, machine_id, volume_id, region, state, token, created_at, last_used
		 FROM user_machines WHERE user_id = ?`, userID)

	var um UserMachine
	var createdAt, lastUsed string
	err := row.Scan(&um.UserID, &um.MachineID, &um.VolumeID, &um.Region, &um.State, &um.Token, &createdAt, &lastUsed)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user machine: %w", err)
	}

	um.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	um.LastUsed, _ = time.Parse(timeFormat, lastUsed)
	return &um, nil
}

// SaveUserMachine upserts a user machine record.
func (s *SQLiteStore) SaveUserMachine(m *UserMachine) error {
	_, err := s.db.Exec(`
		INSERT INTO user_machines (user_id, machine_id, volume_id, region, state, token, created_at, last_used)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			machine_id = excluded.machine_id,
			volume_id  = excluded.volume_id,
			region     = excluded.region,
			state      = excluded.state,
			token      = excluded.token,
			created_at = excluded.created_at,
			last_used  = excluded.last_used
	`, m.UserID, m.MachineID, m.VolumeID, m.Region, m.State, m.Token,
		m.CreatedAt.UTC().Format(timeFormat),
		m.LastUsed.UTC().Format(timeFormat))
	return err
}

// DeleteUserMachine removes a user's machine record from the store.
func (s *SQLiteStore) DeleteUserMachine(userID string) error {
	_, err := s.db.Exec(`DELETE FROM user_machines WHERE user_id = ?`, userID)
	return err
}

// ListAllMachines returns all machine records in the store.
func (s *SQLiteStore) ListAllMachines() ([]*UserMachine, error) {
	rows, err := s.db.Query(
		`SELECT user_id, machine_id, volume_id, region, state, token, created_at, last_used
		 FROM user_machines`)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}
	defer rows.Close()

	var machines []*UserMachine
	for rows.Next() {
		var um UserMachine
		var createdAt, lastUsed string
		if err := rows.Scan(&um.UserID, &um.MachineID, &um.VolumeID, &um.Region, &um.State, &um.Token, &createdAt, &lastUsed); err != nil {
			return nil, fmt.Errorf("scan machine: %w", err)
		}
		um.CreatedAt, _ = time.Parse(timeFormat, createdAt)
		um.LastUsed, _ = time.Parse(timeFormat, lastUsed)
		machines = append(machines, &um)
	}
	return machines, rows.Err()
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
