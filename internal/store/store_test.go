package store

import (
	"os"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := NewWithPath(dbPath, "test-secret-key-for-encryption")
	if err != nil {
		t.Fatalf("NewWithPath: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewWithPath(t *testing.T) {
	s := testStore(t)
	if s.db == nil {
		t.Fatal("db should not be nil")
	}
}

func TestGetUserNotFound(t *testing.T) {
	s := testStore(t)
	u, err := s.GetUser(99999)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u != nil {
		t.Error("Expected nil for non-existent user")
	}
}

func TestSaveAndGetUser(t *testing.T) {
	s := testStore(t)

	enc, err := s.EncryptKey("sk-test-key-123")
	if err != nil {
		t.Fatalf("EncryptKey: %v", err)
	}

	u := &User{
		TelegramUserID: 12345,
		Backend:        "claude",
		APIKeyEnc:      enc,
		Model:          "claude-sonnet-4-20250514",
		AIEnabled:      true,
	}
	if err := s.SaveUser(u); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	got, err := s.GetUser(12345)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got == nil {
		t.Fatal("Expected user, got nil")
	}
	if got.Backend != "claude" {
		t.Errorf("Backend = %q, want claude", got.Backend)
	}
	if got.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want claude-sonnet-4-20250514", got.Model)
	}
	if !got.AIEnabled {
		t.Error("AIEnabled should be true")
	}

	// Verify decryption
	key, err := s.DecryptKey(got.APIKeyEnc)
	if err != nil {
		t.Fatalf("DecryptKey: %v", err)
	}
	if key != "sk-test-key-123" {
		t.Errorf("Decrypted key = %q, want sk-test-key-123", key)
	}
}

func TestSaveUserUpsert(t *testing.T) {
	s := testStore(t)

	u := &User{
		TelegramUserID: 100,
		Backend:        "claude",
		AIEnabled:      true,
	}
	if err := s.SaveUser(u); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	u.Backend = "openai"
	u.Model = "gpt-4"
	if err := s.SaveUser(u); err != nil {
		t.Fatalf("SaveUser (upsert): %v", err)
	}

	got, err := s.GetUser(100)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.Backend != "openai" {
		t.Errorf("Backend = %q, want openai", got.Backend)
	}
	if got.Model != "gpt-4" {
		t.Errorf("Model = %q, want gpt-4", got.Model)
	}
}

func TestDeleteAPIKey(t *testing.T) {
	s := testStore(t)

	enc, _ := s.EncryptKey("sk-secret")
	u := &User{
		TelegramUserID: 200,
		Backend:        "claude",
		APIKeyEnc:      enc,
		AIEnabled:      true,
	}
	s.SaveUser(u)

	if err := s.DeleteAPIKey(200); err != nil {
		t.Fatalf("DeleteAPIKey: %v", err)
	}

	got, _ := s.GetUser(200)
	if got.APIKeyEnc != "" {
		t.Error("API key should be empty after delete")
	}
	if got.AIEnabled {
		t.Error("AIEnabled should be false after key delete")
	}
}

func TestUpdateBackend(t *testing.T) {
	s := testStore(t)

	s.SaveUser(&User{TelegramUserID: 300, Backend: "claude"})

	if err := s.UpdateBackend(300, "gemini"); err != nil {
		t.Fatalf("UpdateBackend: %v", err)
	}

	got, _ := s.GetUser(300)
	if got.Backend != "gemini" {
		t.Errorf("Backend = %q, want gemini", got.Backend)
	}
}

func TestUpdateModel(t *testing.T) {
	s := testStore(t)

	s.SaveUser(&User{TelegramUserID: 400, Backend: "openai"})

	if err := s.UpdateModel(400, "gpt-4o"); err != nil {
		t.Fatalf("UpdateModel: %v", err)
	}

	got, _ := s.GetUser(400)
	if got.Model != "gpt-4o" {
		t.Errorf("Model = %q, want gpt-4o", got.Model)
	}
}

func TestEncryptDecryptKey(t *testing.T) {
	s := testStore(t)

	keys := []string{
		"sk-ant-api03-xxxxxxxxxxxx",
		"sk-proj-abcdef123456",
		"AIzaSyxxxxxxxxxxxxxxxxxxxxxxx",
		"",
	}

	for _, key := range keys {
		if key == "" {
			dec, err := s.DecryptKey("")
			if err != nil {
				t.Fatalf("DecryptKey empty: %v", err)
			}
			if dec != "" {
				t.Error("Empty encrypted should return empty")
			}
			continue
		}

		enc, err := s.EncryptKey(key)
		if err != nil {
			t.Fatalf("EncryptKey(%q): %v", key, err)
		}
		if enc == key {
			t.Errorf("Encrypted key should differ from plaintext")
		}

		dec, err := s.DecryptKey(enc)
		if err != nil {
			t.Fatalf("DecryptKey: %v", err)
		}
		if dec != key {
			t.Errorf("DecryptKey = %q, want %q", dec, key)
		}
	}
}

func TestEncryptKeyDifferentNonces(t *testing.T) {
	s := testStore(t)

	enc1, _ := s.EncryptKey("same-key")
	enc2, _ := s.EncryptKey("same-key")

	if enc1 == enc2 {
		t.Error("Two encryptions of the same key should produce different ciphertexts (different nonces)")
	}

	// Both should decrypt to the same value
	dec1, _ := s.DecryptKey(enc1)
	dec2, _ := s.DecryptKey(enc2)
	if dec1 != dec2 {
		t.Error("Both should decrypt to the same plaintext")
	}
}

func TestUpdateMode(t *testing.T) {
	s := testStore(t)

	s.SaveUser(&User{TelegramUserID: 500, Backend: "claude"})

	if err := s.UpdateMode(500, ModeAPI); err != nil {
		t.Fatalf("UpdateMode: %v", err)
	}

	got, _ := s.GetUser(500)
	if got.Mode != ModeAPI {
		t.Errorf("Mode = %q, want %q", got.Mode, ModeAPI)
	}

	if err := s.UpdateMode(500, ModeCLI); err != nil {
		t.Fatalf("UpdateMode: %v", err)
	}

	got, _ = s.GetUser(500)
	if got.Mode != ModeCLI {
		t.Errorf("Mode = %q, want %q", got.Mode, ModeCLI)
	}
}

func TestDefaultModeCLI(t *testing.T) {
	s := testStore(t)

	// Save user without explicit mode
	s.SaveUser(&User{TelegramUserID: 600, Backend: "claude"})

	got, _ := s.GetUser(600)
	if got.Mode != ModeCLI {
		t.Errorf("Default Mode = %q, want %q", got.Mode, ModeCLI)
	}
}

func TestSaveUserWithMode(t *testing.T) {
	s := testStore(t)

	u := &User{
		TelegramUserID: 700,
		Backend:        "openai",
		Mode:           ModeAPI,
		AIEnabled:      true,
	}
	s.SaveUser(u)

	got, _ := s.GetUser(700)
	if got.Mode != ModeAPI {
		t.Errorf("Mode = %q, want %q", got.Mode, ModeAPI)
	}
	if got.Backend != "openai" {
		t.Errorf("Backend = %q, want openai", got.Backend)
	}
}

func TestCLIBinaryForBackend(t *testing.T) {
	tests := []struct {
		backend string
		want    string
	}{
		{"claude", "claude"},
		{"openai", "codex"},
		{"gemini", "gemini"},
		{"ollama", "ollama"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := CLIBinaryForBackend(tt.backend)
		if got != tt.want {
			t.Errorf("CLIBinaryForBackend(%q) = %q, want %q", tt.backend, got, tt.want)
		}
	}
}

func TestNewCreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	s, err := New("test-key")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	dbPath := filepath.Join(tmpDir, ".same", "telegram-users.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file should exist")
	}
}
