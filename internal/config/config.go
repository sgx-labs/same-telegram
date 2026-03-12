package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds all same-telegram configuration.
type Config struct {
	Bot    BotConfig    `toml:"bot"`
	AI     AIConfig     `toml:"ai"`
	Notify NotifyConfig `toml:"notify"`
	Digest DigestConfig `toml:"digest"`
	Watch  WatchConfig  `toml:"watch"`
}

// AIConfig holds AI-related settings.
type AIConfig struct {
	DailyLimit int `toml:"daily_limit"` // 0 = unlimited
}

// BotConfig holds Telegram bot settings.
type BotConfig struct {
	Token                string  `toml:"token"`
	AllowedUserIDs       []int64 `toml:"allowed_user_ids"`
	EncryptionKey        string  `toml:"encryption_key"`
	OwnerID              int64   `toml:"owner_id"`
	DangerousPermissions bool    `toml:"dangerous_permissions"`
	Mode                 string  `toml:"mode"` // "internal", "public", or "workspace"

	// Workspace mode settings.
	InviteCode    string `toml:"invite_code"`     // optional invite code for registration
	WorkspaceHost string `toml:"workspace_host"`  // hostname for Mini App (e.g., "workspace.samevault.com")
	MaxUsers      int    `toml:"max_users"`        // max registered users (0 = unlimited)

	// Fly Machines API settings (workspace mode).
	FlyAPIToken   string `toml:"fly_api_token"`   // Fly Machines API token
	FlyAppName    string `toml:"fly_app_name"`    // Fly app name for workspace machines
	FlyRegion     string `toml:"fly_region"`      // default region (e.g., "iad")
	FlyImage      string `toml:"fly_image"`       // Docker image for workspace containers
	MachineDBPath string `toml:"machine_db_path"` // path to machine store SQLite DB
}

// IsWorkspaceMode returns true when the bot is running in workspace mode.
func (b *BotConfig) IsWorkspaceMode() bool {
	return b.Mode == "workspace"
}

// IsPublicMode returns true when the bot is running in public (restricted) mode.
func (b *BotConfig) IsPublicMode() bool {
	return b.Mode == "public"
}

// EffectiveOwnerID returns the configured owner_id, or falls back to the first
// entry in allowed_user_ids if owner_id is not set.
func (b *BotConfig) EffectiveOwnerID() int64 {
	if b.OwnerID != 0 {
		return b.OwnerID
	}
	if len(b.AllowedUserIDs) > 0 {
		return b.AllowedUserIDs[0]
	}
	return 0
}

// NotifyConfig controls which notifications are sent.
type NotifyConfig struct {
	SessionEnd bool `toml:"session_end"`
	Decisions  bool `toml:"decisions"`
	Handoffs   bool `toml:"handoffs"`
}

// DigestConfig controls the daily digest.
type DigestConfig struct {
	Enabled bool   `toml:"enabled"`
	Time    string `toml:"time"` // HH:MM in local time
}

// WatchConfig controls file-watching for data directories.
type WatchConfig struct {
	Enabled   bool              `toml:"enabled"`
	ExtraDirs map[string]string `toml:"extra_dirs"` // path -> category (review/decision/report)
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		AI: AIConfig{
			DailyLimit: 100,
		},
		Notify: NotifyConfig{
			SessionEnd: true,
			Decisions:  true,
			Handoffs:   true,
		},
		Digest: DigestConfig{
			Enabled: true,
			Time:    "08:00",
		},
		Watch: WatchConfig{
			Enabled: true,
		},
	}
}

// OverrideConfigPath allows setting a custom config file path via --config flag.
var OverrideConfigPath string

// configDir returns ~/.same/
func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".same")
}

// ConfigPath returns the path to the telegram config file.
func ConfigPath() string {
	if OverrideConfigPath != "" {
		return OverrideConfigPath
	}
	return filepath.Join(configDir(), "telegram.toml")
}

// pathSuffix returns a suffix derived from the config file name for multi-instance support.
// e.g., "telegram-public.toml" -> "-public", "telegram.toml" -> ""
func pathSuffix() string {
	if OverrideConfigPath == "" {
		return ""
	}
	base := filepath.Base(OverrideConfigPath)
	base = strings.TrimSuffix(base, filepath.Ext(base)) // "telegram-public"
	if base == "telegram" {
		return ""
	}
	return strings.TrimPrefix(base, "telegram") // "-public"
}

// SocketPath returns the path to the daemon unix socket.
func SocketPath() string {
	return filepath.Join(configDir(), "telegram"+pathSuffix()+".sock")
}

// PidPath returns the path to the daemon PID file.
func PidPath() string {
	return filepath.Join(configDir(), "telegram"+pathSuffix()+".pid")
}

// LogPath returns the path to the daemon log file.
func LogPath() string {
	return filepath.Join(configDir(), "telegram"+pathSuffix()+".log")
}

// Load reads the config from ~/.same/telegram.toml.
func Load() (*Config, error) {
	path := ConfigPath()
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file — allow pure env-var configuration.
			applyEnvOverrides(&cfg.Bot)
			if cfg.Bot.Token == "" {
				return nil, fmt.Errorf("config not found at %s and TELEGRAM_BOT_TOKEN not set", path)
			}
			// Skip TOML parsing, proceed to validation below.
		} else {
			return nil, fmt.Errorf("read config: %w", err)
		}
	} else {
		md, err := toml.Decode(string(data), cfg)
		if err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}

		// Warn about unknown keys
		for _, key := range md.Undecoded() {
			fmt.Fprintf(os.Stderr, "same-telegram: WARNING: unknown key %q in %s\n", key.String(), path)
		}

		// Environment variable overrides (deploy-time overrides for TOML defaults).
		applyEnvOverrides(&cfg.Bot)
	}

	if cfg.Bot.Token == "" {
		return nil, fmt.Errorf("bot.token is required (set in %s or TELEGRAM_BOT_TOKEN env var)", path)
	}
	if len(cfg.Bot.AllowedUserIDs) == 0 && !cfg.Bot.IsPublicMode() && !cfg.Bot.IsWorkspaceMode() {
		return nil, fmt.Errorf("bot.allowed_user_ids is required in %s (security: whitelist your Telegram user ID)", path)
	}

	// Workspace mode requires Fly API credentials.
	if cfg.Bot.IsWorkspaceMode() {
		if cfg.Bot.FlyAPIToken == "" {
			return nil, fmt.Errorf("bot.fly_api_token is required in workspace mode (set in %s or FLY_API_TOKEN env var)", path)
		}
		if cfg.Bot.FlyAppName == "" {
			return nil, fmt.Errorf("bot.fly_app_name is required in workspace mode (set in %s or FLY_APP_NAME env var)", path)
		}
	}

	return cfg, nil
}

// applyEnvOverrides applies environment variable overrides to BotConfig.
// Env vars take precedence over TOML values when set.
func applyEnvOverrides(bot *BotConfig) {
	if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
		bot.Token = v
	}
	if v := os.Getenv("BOT_MODE"); v != "" {
		bot.Mode = v
	}
	if v := os.Getenv("WORKSPACE_HOST"); v != "" {
		bot.WorkspaceHost = v
	}
	if v := os.Getenv("INVITE_CODE"); v != "" {
		bot.InviteCode = v
	}
	if v := os.Getenv("ALLOWED_USER_IDS"); v != "" {
		var ids []int64
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if id, err := strconv.ParseInt(s, 10, 64); err == nil {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			bot.AllowedUserIDs = ids
		}
	}
	if v := os.Getenv("WORKSPACE_FLY_TOKEN"); v != "" {
		bot.FlyAPIToken = v
	} else if v := os.Getenv("FLY_API_TOKEN"); v != "" {
		bot.FlyAPIToken = v
	}
	if v := os.Getenv("WORKSPACE_APP_NAME"); v != "" {
		bot.FlyAppName = v
	} else if v := os.Getenv("FLY_APP_NAME"); v != "" {
		bot.FlyAppName = v
	}
	if v := os.Getenv("FLY_REGION"); v != "" {
		bot.FlyRegion = v
	}
	if v := os.Getenv("FLY_IMAGE"); v != "" {
		bot.FlyImage = v
	}
	if v := os.Getenv("MACHINE_DB_PATH"); v != "" {
		bot.MachineDBPath = v
	}

	// Apply defaults for fields that are still empty.
	if bot.FlyRegion == "" {
		bot.FlyRegion = "iad"
	}
	if bot.MachineDBPath == "" {
		bot.MachineDBPath = "~/.same/machines.db"
	}
}

// EncryptionKeyPath returns the path to the auto-generated encryption key file.
func EncryptionKeyPath() string {
	return filepath.Join(configDir(), "telegram-encryption.key")
}

// LoadOrGenerateEncryptionKey returns an encryption key for the store.
// If configKey is non-empty, it is used directly.
// Otherwise, a key is loaded from ~/.same/telegram-encryption.key.
// If that file does not exist, a random 32-byte key is generated, saved
// to that file (mode 0600), and returned.
func LoadOrGenerateEncryptionKey(configKey string) (string, error) {
	if configKey != "" {
		return configKey, nil
	}

	keyPath := EncryptionKeyPath()

	// Try to load existing key file
	data, err := os.ReadFile(keyPath)
	if err == nil {
		key := strings.TrimSpace(string(data))
		if key != "" {
			return key, nil
		}
	}

	// Generate a new random key
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate encryption key: %w", err)
	}
	key := hex.EncodeToString(b)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return "", fmt.Errorf("create config dir for encryption key: %w", err)
	}

	// Write key file with restrictive permissions
	if err := os.WriteFile(keyPath, []byte(key+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("save encryption key: %w", err)
	}

	return key, nil
}

// generateRandomKey generates a random hex-encoded encryption key.
func generateRandomKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "change-me-to-a-random-secret"
	}
	return hex.EncodeToString(b)
}

// GenerateTemplate writes a commented config template to ~/.same/telegram.toml.
func GenerateTemplate(token string, userID int64) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	encKey := generateRandomKey()

	content := fmt.Sprintf(`# same-telegram configuration
# Generated by: same-telegram setup

[bot]
# Telegram bot token from @BotFather
token = %q

# Telegram user IDs allowed to interact with this bot.
# Only these users can send commands. Others are silently ignored.
# Find your ID: message @userinfobot on Telegram
allowed_user_ids = [%d]

# Owner user ID — only this user can use CLI mode (local code execution).
# Defaults to the first entry in allowed_user_ids if not set.
# owner_id = %d

# Encryption key for API keys stored at rest (AES-256-GCM).
# Change this to a random string. If you change it, existing keys must be re-entered.
encryption_key = %q

[notify]
# Which notifications to send to Telegram
session_end = true
decisions = true
handoffs = true

[digest]
# Daily digest of vault activity
enabled = true
time = "08:00"  # HH:MM in local time

[watch]
# Watch data directories for new review/decision/report files
enabled = true
# Add extra directories to watch (path = category):
# [watch.extra_dirs]
# "/path/to/custom/dir" = "review"
`, token, userID, userID, encKey)

	path := ConfigPath()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
