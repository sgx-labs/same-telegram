package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Notify.SessionEnd {
		t.Error("SessionEnd should default to true")
	}
	if !cfg.Notify.Decisions {
		t.Error("Decisions should default to true")
	}
	if !cfg.Digest.Enabled {
		t.Error("Digest should default to enabled")
	}
	if cfg.Digest.Time != "08:00" {
		t.Errorf("Digest time should be 08:00, got %s", cfg.Digest.Time)
	}
}

func TestGenerateTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	// Override configDir for test
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	err := GenerateTemplate("123:ABC", 12345)
	if err != nil {
		t.Fatalf("GenerateTemplate: %v", err)
	}

	path := filepath.Join(tmpDir, ".same", "telegram.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Read config: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Error("Config file is empty")
	}
}

func TestLoadMissing(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	_, err := Load()
	if err == nil {
		t.Error("Expected error for missing config")
	}
}

func TestLoadValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	configContent := `
[bot]
token = "123:ABC"
allowed_user_ids = [12345]

[notify]
session_end = true
decisions = false

[digest]
enabled = true
time = "09:00"
`
	dir := filepath.Join(tmpDir, ".same")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "telegram.toml"), []byte(configContent), 0o600)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Bot.Token != "123:ABC" {
		t.Errorf("Token = %q, want 123:ABC", cfg.Bot.Token)
	}
	if len(cfg.Bot.AllowedUserIDs) != 1 || cfg.Bot.AllowedUserIDs[0] != 12345 {
		t.Errorf("AllowedUserIDs = %v, want [12345]", cfg.Bot.AllowedUserIDs)
	}
	if cfg.Notify.Decisions {
		t.Error("Decisions should be false")
	}
	if cfg.Digest.Time != "09:00" {
		t.Errorf("Digest.Time = %q, want 09:00", cfg.Digest.Time)
	}
}
