package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Connection mode constants.
const (
	ModeAPI = "api" // use API key
	ModeCLI = "cli" // use local CLI binary
)

// User represents a Telegram user's AI configuration.
type User struct {
	TelegramUserID int64
	Backend        string
	APIKeyEnc      string // hex-encoded encrypted key
	Model          string
	Mode           string // "api" or "cli"
	AIEnabled      bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Store manages persistent user state in SQLite.
type Store struct {
	db        *sql.DB
	encKey    []byte // 32-byte AES-256 key derived from config
}

// hkdfSalt is a fixed salt for key derivation. Changing this invalidates all
// previously encrypted keys.
var hkdfSalt = []byte("same-telegram-store-v1")

// hkdfInfo is the context info for HKDF-Expand.
var hkdfInfo = []byte("aes-256-gcm-encryption-key")

// deriveKey derives a 32-byte AES-256 key from the encryption secret using
// HKDF (RFC 5869) with SHA-256. This replaces the previous bare SHA-256 hash,
// which lacked salt and was weaker against brute-force attacks.
//
// NOTE: This is a breaking change — keys encrypted with the old SHA-256
// derivation will fail to decrypt. On first run after upgrade, users must
// re-enter their API keys via /onboard.
func deriveKey(secret string) []byte {
	return hkdfSHA256([]byte(secret), hkdfSalt, hkdfInfo, 32)
}

// deriveKeyLegacy derives a key using the old SHA-256 method (for migration).
func deriveKeyLegacy(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// hkdfSHA256 implements HKDF-Extract + HKDF-Expand (RFC 5869) using HMAC-SHA256.
func hkdfSHA256(ikm, salt, info []byte, length int) []byte {
	// HKDF-Extract: PRK = HMAC-SHA256(salt, IKM)
	if len(salt) == 0 {
		salt = make([]byte, sha256.Size)
	}
	extractor := hmac.New(sha256.New, salt)
	extractor.Write(ikm)
	prk := extractor.Sum(nil)

	// HKDF-Expand: OKM = T(1) || T(2) || ... truncated to length
	var okm []byte
	var prev []byte
	for i := byte(1); len(okm) < length; i++ {
		expander := hmac.New(sha256.New, prk)
		expander.Write(prev)
		expander.Write(info)
		expander.Write([]byte{i})
		prev = expander.Sum(nil)
		okm = append(okm, prev...)
	}
	return okm[:length]
}

// New opens (or creates) the SQLite database and returns a Store.
func New(encryptionKey string) (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	dir := filepath.Join(home, ".same")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}

	dbPath := filepath.Join(dir, "telegram-users.db")
	return NewWithPath(dbPath, encryptionKey)
}

// NewWithPath opens a Store at a specific path (useful for testing).
func NewWithPath(dbPath, encryptionKey string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Restrict database file permissions to owner-only (0600).
	// The file may already exist; always enforce restrictive permissions.
	if err := os.Chmod(dbPath, 0o600); err != nil && !os.IsNotExist(err) {
		// Log but don't fail — the file may not exist yet (sqlite creates on first write).
		_ = err
	}

	s := &Store{
		db:     db,
		encKey: deriveKey(encryptionKey),
	}

	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	// Ensure permissions after migration (sqlite may have created the file).
	_ = os.Chmod(dbPath, 0o600)

	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			telegram_user_id INTEGER PRIMARY KEY,
			backend          TEXT NOT NULL DEFAULT '',
			api_key_encrypted TEXT NOT NULL DEFAULT '',
			model            TEXT NOT NULL DEFAULT '',
			mode             TEXT NOT NULL DEFAULT 'cli',
			ai_enabled       INTEGER NOT NULL DEFAULT 0,
			created_at       TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at       TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		return err
	}

	// Migration: add mode column if it doesn't exist (for existing databases).
	s.db.Exec(`ALTER TABLE users ADD COLUMN mode TEXT NOT NULL DEFAULT 'cli'`)

	// Migration: usage tracking table.
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS usage (
			telegram_user_id INTEGER NOT NULL,
			date             TEXT NOT NULL,
			messages_today   INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (telegram_user_id, date)
		)
	`)
	if err != nil {
		return err
	}

	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// GetUser retrieves a user by Telegram user ID. Returns nil, nil if not found.
func (s *Store) GetUser(telegramUserID int64) (*User, error) {
	row := s.db.QueryRow(
		`SELECT telegram_user_id, backend, api_key_encrypted, model, mode, ai_enabled, created_at, updated_at
		 FROM users WHERE telegram_user_id = ?`, telegramUserID)

	var u User
	var aiEnabled int
	var createdAt, updatedAt string
	err := row.Scan(&u.TelegramUserID, &u.Backend, &u.APIKeyEnc, &u.Model, &u.Mode, &aiEnabled, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	u.AIEnabled = aiEnabled != 0
	if u.Mode == "" {
		u.Mode = ModeCLI
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	u.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	return &u, nil
}

// SaveUser upserts a user record.
func (s *Store) SaveUser(u *User) error {
	mode := u.Mode
	if mode == "" {
		mode = ModeCLI
	}
	_, err := s.db.Exec(`
		INSERT INTO users (telegram_user_id, backend, api_key_encrypted, model, mode, ai_enabled, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(telegram_user_id) DO UPDATE SET
			backend = excluded.backend,
			api_key_encrypted = excluded.api_key_encrypted,
			model = excluded.model,
			mode = excluded.mode,
			ai_enabled = excluded.ai_enabled,
			updated_at = datetime('now')
	`, u.TelegramUserID, u.Backend, u.APIKeyEnc, u.Model, mode, boolToInt(u.AIEnabled))
	return err
}

// DeleteAPIKey removes the stored API key for a user.
func (s *Store) DeleteAPIKey(telegramUserID int64) error {
	_, err := s.db.Exec(
		`UPDATE users SET api_key_encrypted = '', ai_enabled = 0, updated_at = datetime('now')
		 WHERE telegram_user_id = ?`, telegramUserID)
	return err
}

// UpdateBackend changes the AI backend for a user.
func (s *Store) UpdateBackend(telegramUserID int64, backend string) error {
	_, err := s.db.Exec(
		`UPDATE users SET backend = ?, updated_at = datetime('now')
		 WHERE telegram_user_id = ?`, backend, telegramUserID)
	return err
}

// UpdateModel changes the AI model for a user.
func (s *Store) UpdateModel(telegramUserID int64, model string) error {
	_, err := s.db.Exec(
		`UPDATE users SET model = ?, updated_at = datetime('now')
		 WHERE telegram_user_id = ?`, model, telegramUserID)
	return err
}

// UpdateMode changes the connection mode for a user ("api" or "cli").
func (s *Store) UpdateMode(telegramUserID int64, mode string) error {
	_, err := s.db.Exec(
		`UPDATE users SET mode = ?, updated_at = datetime('now')
		 WHERE telegram_user_id = ?`, mode, telegramUserID)
	return err
}

// CLIBinaryForBackend returns the expected CLI binary name for a backend.
func CLIBinaryForBackend(backend string) string {
	switch backend {
	case "claude":
		return "claude"
	case "openai":
		return "codex"
	case "gemini":
		return "gemini"
	case "ollama":
		return "ollama"
	default:
		return backend
	}
}

// EncryptKey encrypts an API key using AES-256-GCM.
func (s *Store) EncryptKey(plaintext string) (string, error) {
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// DecryptKey decrypts an API key from its hex-encoded AES-256-GCM ciphertext.
func (s *Store) DecryptKey(encHex string) (string, error) {
	if encHex == "" {
		return "", nil
	}

	data, err := hex.DecodeString(encHex)
	if err != nil {
		return "", fmt.Errorf("decode hex: %w", err)
	}

	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// todayString returns today's date as YYYY-MM-DD in UTC.
func todayString() string {
	return time.Now().UTC().Format("2006-01-02")
}

// IncrementMessageCount increments today's message counter for a user.
// It automatically resets the count on a new day by using the date as part of the key.
func (s *Store) IncrementMessageCount(telegramUserID int64) (int, error) {
	today := todayString()
	_, err := s.db.Exec(`
		INSERT INTO usage (telegram_user_id, date, messages_today)
		VALUES (?, ?, 1)
		ON CONFLICT(telegram_user_id, date) DO UPDATE SET
			messages_today = messages_today + 1
	`, telegramUserID, today)
	if err != nil {
		return 0, fmt.Errorf("increment message count: %w", err)
	}

	return s.GetMessageCount(telegramUserID)
}

// GetMessageCount returns today's message count for a user.
func (s *Store) GetMessageCount(telegramUserID int64) (int, error) {
	today := todayString()
	var count int
	err := s.db.QueryRow(
		`SELECT messages_today FROM usage WHERE telegram_user_id = ? AND date = ?`,
		telegramUserID, today).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get message count: %w", err)
	}
	return count, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
