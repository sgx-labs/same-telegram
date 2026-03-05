package filter

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// redactPlaceholder is the replacement text for matched PII.
const redactPlaceholder = "[REDACTED]"

// defaultPatterns are compiled into the binary as a baseline.
var defaultPatterns = []string{
	// Email addresses
	`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`,

	// IPv4 addresses (but not version-like strings such as 1.2.3)
	`\b(?:\d{1,3}\.){3}\d{1,3}\b`,

	// IPv6 addresses (simplified: 4+ hex groups with colons)
	`\b(?:[0-9a-fA-F]{1,4}:){3,7}[0-9a-fA-F]{1,4}\b`,

	// Local file paths: /Users/*, /home/*, C:\Users\*
	`(?:/Users/|/home/)[^\s"'` + "`" + `\x00]+`,
	`[A-Z]:\\\\Users\\\\[^\s"'` + "`" + `\x00]+`,

	// API keys / tokens: long hex strings (32+ chars)
	`\b[0-9a-fA-F]{32,}\b`,

	// Base64-encoded tokens (40+ chars, no spaces)
	`\b[A-Za-z0-9+/]{40,}={0,2}\b`,

	// Common API key prefixes
	`\bsk-ant-api[A-Za-z0-9\-]+`,
	`\bsk-proj-[A-Za-z0-9]{20,}`,
	`\bAIzaSy[A-Za-z0-9]{30,}`,
	`\bghp_[A-Za-z0-9]{36,}`,
	`\bglpat-[A-Za-z0-9\-]{20,}`,
	`\bxoxb-[A-Za-z0-9\-]+`,
	`\bxoxp-[A-Za-z0-9\-]+`,

	// AWS-style keys
	`\bAKIA[0-9A-Z]{16}\b`,
	`\bASIA[0-9A-Z]{16}\b`,

	// Telegram bot tokens (numeric:alphanumeric, suffix is typically 34-35 chars)
	`\b\d{8,10}:[A-Za-z0-9_-]{34,}\b`,

	// Telegram user IDs (bare 8-10 digit numbers preceded by "user_id" or "USER_ID" etc.)
	`(?i)(?:user.?id|chat.?id|ceo.?id|allowed.?user)[^\d]{0,5}\d{6,12}`,

	// Generic secret patterns (KEY=value, TOKEN=value, SECRET=value)
	`(?i)(?:token|secret|password|api_key|apikey|bot_token)\s*[=:]\s*\S+`,

	// SSN (US)
	`\b\d{3}-\d{2}-\d{4}\b`,

	// Phone numbers (US-style with area code)
	`\b(?:\+1[\s\-]?)?\(?\d{3}\)?[\s\-]?\d{3}[\s\-]?\d{4}\b`,
}

// Filter holds compiled regex patterns for PII sanitization.
type Filter struct {
	mu       sync.RWMutex
	patterns []*regexp.Regexp
}

// New creates a Filter with default patterns. It also loads any additional
// patterns from ~/.same/blocklist.txt if that file exists.
func New() *Filter {
	f := &Filter{}
	f.loadDefaults()
	f.loadBlocklistFile()
	return f
}

// NewWithPatterns creates a Filter using only the provided regex pattern strings.
// Useful for testing.
func NewWithPatterns(patterns []string) *Filter {
	f := &Filter{}
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			f.patterns = append(f.patterns, re)
		}
	}
	return f
}

// Sanitize replaces all PII matches in text with [REDACTED].
func (f *Filter) Sanitize(text string) string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	for _, re := range f.patterns {
		text = re.ReplaceAllString(text, redactPlaceholder)
	}
	return text
}

// AddPattern compiles and adds a new regex pattern at runtime.
func (f *Filter) AddPattern(pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.patterns = append(f.patterns, re)
	f.mu.Unlock()
	return nil
}

func (f *Filter) loadDefaults() {
	for _, p := range defaultPatterns {
		if re, err := regexp.Compile(p); err == nil {
			f.patterns = append(f.patterns, re)
		}
	}
}

func (f *Filter) loadBlocklistFile() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".same", "blocklist.txt")
	file, err := os.Open(path)
	if err != nil {
		return // file is optional
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if re, err := regexp.Compile(line); err == nil {
			f.patterns = append(f.patterns, re)
		}
	}
}

// Sanitize is a package-level convenience using the default global filter.
// Prefer creating a Filter instance with New() for production use.
var globalFilter *Filter
var globalOnce sync.Once

func defaultFilter() *Filter {
	globalOnce.Do(func() {
		globalFilter = New()
	})
	return globalFilter
}

// Sanitize sanitizes text using the global default filter.
func Sanitize(text string) string {
	return defaultFilter().Sanitize(text)
}
