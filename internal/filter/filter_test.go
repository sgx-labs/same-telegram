package filter

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSanitizeEmail(t *testing.T) {
	f := New()
	tests := []struct {
		input    string
		contains string
	}{
		{"Contact user@example.com for help", "[REDACTED]"},
		{"Email: admin+tag@sub.domain.org", "[REDACTED]"},
		{"No email here", ""},
	}
	for _, tt := range tests {
		got := f.Sanitize(tt.input)
		if tt.contains != "" && !strings.Contains(got, tt.contains) {
			t.Errorf("Sanitize(%q) = %q, want it to contain %q", tt.input, got, tt.contains)
		}
		if tt.contains != "" && strings.Contains(got, "@") {
			t.Errorf("Sanitize(%q) = %q, email not fully redacted", tt.input, got)
		}
		if tt.contains == "" && got != tt.input {
			t.Errorf("Sanitize(%q) = %q, should be unchanged", tt.input, got)
		}
	}
}

func TestSanitizeIPv4(t *testing.T) {
	f := New()
	tests := []struct {
		input string
		clean bool
	}{
		{"Server at 192.168.1.1 is down", false},
		{"IP: 10.0.0.255", false},
		{"Version 1.2.3 released", true}, // Not an IP (only 3 octets)
	}
	for _, tt := range tests {
		got := f.Sanitize(tt.input)
		hasIP := strings.Contains(got, "192.168") || strings.Contains(got, "10.0.0")
		if !tt.clean && hasIP {
			t.Errorf("Sanitize(%q) = %q, IP not redacted", tt.input, got)
		}
	}
}

func TestSanitizeLocalPaths(t *testing.T) {
	f := New()
	tests := []struct {
		input string
	}{
		{"File at /Users/john/Documents/secret.txt"},
		{"Path: /home/admin/.ssh/id_rsa"},
	}
	for _, tt := range tests {
		got := f.Sanitize(tt.input)
		if strings.Contains(got, "/Users/john") || strings.Contains(got, "/home/admin") {
			t.Errorf("Sanitize(%q) = %q, path not redacted", tt.input, got)
		}
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("Sanitize(%q) = %q, expected [REDACTED]", tt.input, got)
		}
	}
}

func TestSanitizeAPIKeys(t *testing.T) {
	f := New()
	tests := []struct {
		name  string
		input string
	}{
		{"Anthropic key", "key: sk-ant-api03-abc123def456"},
		{"OpenAI key", "sk-proj-ABCDEFGHIJKLMNOPQRST12345"},
		{"Google key", "AIzaSyABCDEFGHIJKLMNOPQRSTUVWXYZ012345"},
		{"GitHub PAT", "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijkl"},
		{"GitLab token", "glpat-ABCDEFGHIJKLMNOPQRSTUVWX"},
		{"Slack bot", "xoxb-123456-789012-abcdef"},
		{"AWS access key", "AKIAIOSFODNN7EXAMPLE"},
		{"Long hex", "Token: a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"},
	}
	for _, tt := range tests {
		got := f.Sanitize(tt.input)
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("%s: Sanitize(%q) = %q, expected redaction", tt.name, tt.input, got)
		}
	}
}

func TestSanitizeSSN(t *testing.T) {
	f := New()
	input := "SSN: 123-45-6789"
	got := f.Sanitize(input)
	if strings.Contains(got, "123-45-6789") {
		t.Errorf("Sanitize(%q) = %q, SSN not redacted", input, got)
	}
}

func TestSanitizePhoneNumber(t *testing.T) {
	f := New()
	tests := []string{
		"Call 555-123-4567",
		"Phone: (555) 123-4567",
		"Reach me at +1 555 123 4567",
	}
	for _, input := range tests {
		got := f.Sanitize(input)
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("Sanitize(%q) = %q, phone not redacted", input, got)
		}
	}
}

func TestSanitizePreservesCleanText(t *testing.T) {
	f := New()
	clean := []string{
		"Hello world",
		"Vault status: 3 entries",
		"Session ended successfully",
		"Decision: use PostgreSQL",
		"Build completed. All tests passed.",
	}
	for _, input := range clean {
		got := f.Sanitize(input)
		if got != input {
			t.Errorf("Sanitize(%q) = %q, clean text should be unchanged", input, got)
		}
	}
}

func TestSanitizeMultiplePII(t *testing.T) {
	f := New()
	input := "User john@example.com at 192.168.1.1 path /Users/john/docs"
	got := f.Sanitize(input)
	if strings.Contains(got, "john@") {
		t.Error("Email not redacted in multi-PII string")
	}
	if strings.Contains(got, "192.168") {
		t.Error("IP not redacted in multi-PII string")
	}
	if strings.Contains(got, "/Users/john") {
		t.Error("Path not redacted in multi-PII string")
	}
}

func TestNewWithPatterns(t *testing.T) {
	f := NewWithPatterns([]string{`secret\d+`})
	got := f.Sanitize("The code is secret42 okay")
	if strings.Contains(got, "secret42") {
		t.Errorf("Custom pattern not applied: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("Expected [REDACTED], got: %q", got)
	}
}

func TestAddPattern(t *testing.T) {
	f := NewWithPatterns(nil)
	input := "project-codename-phoenix"

	// Before adding pattern
	got := f.Sanitize(input)
	if got != input {
		t.Errorf("Before AddPattern, text should be unchanged: %q", got)
	}

	// Add pattern
	if err := f.AddPattern(`codename-\w+`); err != nil {
		t.Fatalf("AddPattern failed: %v", err)
	}

	got = f.Sanitize(input)
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("After AddPattern, expected redaction: %q", got)
	}
}

func TestAddPatternInvalid(t *testing.T) {
	f := NewWithPatterns(nil)
	err := f.AddPattern(`[invalid`)
	if err == nil {
		t.Error("Expected error for invalid regex")
	}
}

func TestPackageLevelSanitize(t *testing.T) {
	got := Sanitize("test user@example.com here")
	if strings.Contains(got, "user@example.com") {
		t.Errorf("Package-level Sanitize did not redact email: %q", got)
	}
}

func TestSanitizeEmptyString(t *testing.T) {
	f := New()
	got := f.Sanitize("")
	if got != "" {
		t.Errorf("Sanitize empty string should return empty, got: %q", got)
	}
}

func TestSanitizeUnicodeWithEmbeddedPII(t *testing.T) {
	f := New()
	tests := []struct {
		name  string
		input string
		leak  string
	}{
		{"CJK with email", "用户联系 user@example.com 获取帮助", "user@example.com"},
		{"emoji with phone", "Call me! 📞 555-123-4567", "555-123-4567"},
		{"RTL with IP", "عنوان 192.168.1.100 الخادم", "192.168.1.100"},
		{"accented with SSN", "resume numero 123-45-6789 du", "123-45-6789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Sanitize(tt.input)
			if strings.Contains(got, tt.leak) {
				t.Errorf("PII not redacted: %q", got)
			}
			if !strings.Contains(got, "[REDACTED]") {
				t.Errorf("Expected [REDACTED], got: %q", got)
			}
		})
	}
}

func TestSanitizePIIInMarkdown(t *testing.T) {
	f := New()
	tests := []struct {
		name  string
		input string
		leak  string
	}{
		{"bold email", "*user@example.com*", "user@example.com"},
		{"code SSN", "`123-45-6789`", "123-45-6789"},
		{"italic path", "_/Users/admin/secret_", "/Users/admin"},
		{"link-like", "[click](http://192.168.1.1)", "192.168.1.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Sanitize(tt.input)
			if strings.Contains(got, tt.leak) {
				t.Errorf("PII in markdown not redacted: %q", got)
			}
		})
	}
}

func TestSanitizeAlreadyRedacted(t *testing.T) {
	f := New()
	input := "Previously [REDACTED] content and *** masked data"
	got := f.Sanitize(input)
	if got != input {
		t.Errorf("Already-redacted text was modified: got %q", got)
	}
}

func TestSanitizeLongString(t *testing.T) {
	f := New()
	// Build a string over Telegram's 4096 char limit with PII scattered throughout
	long := strings.Repeat("This is clean text. ", 200)
	long += " Email: leak@example.com "
	long += strings.Repeat("More clean text. ", 50)
	got := f.Sanitize(long)
	if strings.Contains(got, "leak@example.com") {
		t.Error("Email in long string not redacted")
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Error("Expected [REDACTED] in long string output")
	}
}

func TestSanitizePIIAtBoundaries(t *testing.T) {
	f := New()
	tests := []struct {
		name  string
		input string
		leak  string
	}{
		{"start of string", "user@example.com is here", "user@example.com"},
		{"end of string", "contact user@example.com", "user@example.com"},
		{"only PII", "user@example.com", "user@example.com"},
		{"newline boundary", "line1\nuser@example.com\nline3", "user@example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Sanitize(tt.input)
			if strings.Contains(got, tt.leak) {
				t.Errorf("PII at boundary not redacted: %q", got)
			}
		})
	}
}

func TestSanitizeIPv6(t *testing.T) {
	f := New()
	tests := []struct {
		name  string
		input string
		leak  string
	}{
		{"full IPv6", "addr: 2001:0db8:85a3:0000:0000:8a2e:0370:7334", "2001:0db8"},
		{"short IPv6", "fe80:0000:0000:0000:abcd:1234:5678:9abc", "fe80:0000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Sanitize(tt.input)
			if strings.Contains(got, tt.leak) {
				t.Errorf("IPv6 not redacted: %q", got)
			}
		})
	}
}

func TestSanitizeWindowsPath(t *testing.T) {
	f := New()
	input := `File at C:\\Users\\admin\\Documents\\secret.txt`
	got := f.Sanitize(input)
	if strings.Contains(got, `C:\\Users\\admin`) {
		t.Errorf("Windows path not redacted: %q", got)
	}
}

func TestSanitizeAWSSessionKey(t *testing.T) {
	f := New()
	input := "key: ASIA1234567890ABCDEF"
	got := f.Sanitize(input)
	if strings.Contains(got, "ASIA1234567890ABCDEF") {
		t.Errorf("AWS session key not redacted: %q", got)
	}
}

func TestSanitizeSlackUserToken(t *testing.T) {
	f := New()
	input := "token: xoxp-1234567890-abcdef"
	got := f.Sanitize(input)
	if strings.Contains(got, "xoxp-") {
		t.Errorf("Slack user token not redacted: %q", got)
	}
}

func TestSanitizeConcurrent(t *testing.T) {
	f := New()
	input := "User john@example.com at 192.168.1.1"

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := f.Sanitize(input)
			if strings.Contains(got, "john@") {
				t.Errorf("Email leaked in concurrent call: %q", got)
			}
			if strings.Contains(got, "192.168") {
				t.Errorf("IP leaked in concurrent call: %q", got)
			}
		}()
	}
	wg.Wait()
}

func TestSanitizeConcurrentWithAddPattern(t *testing.T) {
	f := NewWithPatterns([]string{`test\d+`})

	var wg sync.WaitGroup
	// Concurrent reads
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.Sanitize("test123 and test456")
		}()
	}
	// Concurrent write
	wg.Add(1)
	go func() {
		defer wg.Done()
		f.AddPattern(`extra\d+`)
	}()
	wg.Wait()
}

func TestNewWithPatternsIgnoresInvalidRegex(t *testing.T) {
	f := NewWithPatterns([]string{`valid\d+`, `[invalid`, `also-valid`})
	got := f.Sanitize("has valid123 here")
	if strings.Contains(got, "valid123") {
		t.Errorf("Valid pattern not applied when mixed with invalid: %q", got)
	}
}

func TestNewWithPatternsNil(t *testing.T) {
	f := NewWithPatterns(nil)
	got := f.Sanitize("user@example.com")
	// No patterns loaded, so nothing should be redacted
	if got != "user@example.com" {
		t.Errorf("Nil patterns should not redact anything: %q", got)
	}
}

func TestNewWithPatternsEmpty(t *testing.T) {
	f := NewWithPatterns([]string{})
	got := f.Sanitize("user@example.com")
	if got != "user@example.com" {
		t.Errorf("Empty patterns should not redact anything: %q", got)
	}
}

func TestBlocklistFile(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create blocklist file
	sameDir := filepath.Join(tmpDir, ".same")
	os.MkdirAll(sameDir, 0o755)
	blocklist := "# Comment line\n\nsecretproject\\d+\n# Another comment\ncodeword-\\w+\n"
	os.WriteFile(filepath.Join(sameDir, "blocklist.txt"), []byte(blocklist), 0o644)

	f := New()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"matches blocklist pattern 1", "The secretproject42 is live", "[REDACTED]"},
		{"matches blocklist pattern 2", "Use codeword-alpha for access", "[REDACTED]"},
		{"no match", "Normal text here", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Sanitize(tt.input)
			if tt.expected != "" && !strings.Contains(got, tt.expected) {
				t.Errorf("Blocklist pattern not applied: got %q", got)
			}
			if tt.expected == "" && got != tt.input {
				t.Errorf("Clean text changed: got %q", got)
			}
		})
	}
}

func TestBlocklistFileNotPresent(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// No blocklist file -- should not error
	f := New()
	got := f.Sanitize("user@example.com")
	if strings.Contains(got, "user@example.com") {
		t.Errorf("Default patterns should still work without blocklist: %q", got)
	}
}

func TestRedactPlaceholderConstant(t *testing.T) {
	if redactPlaceholder != "[REDACTED]" {
		t.Errorf("redactPlaceholder = %q, want [REDACTED]", redactPlaceholder)
	}
}

func TestSanitizePreservesNonPIINumbers(t *testing.T) {
	f := New()
	clean := []string{
		"Version 2.1.0 released",
		"Task #42 completed",
		"3 sessions today",
		"Port 8080 is open",
		"Error code 404",
	}
	for _, input := range clean {
		got := f.Sanitize(input)
		if got != input {
			t.Errorf("Non-PII number text changed: %q -> %q", input, got)
		}
	}
}

func TestSanitizePartialEmailNoTLD(t *testing.T) {
	f := New()
	// "user@example" without a TLD should NOT be caught by the email pattern,
	// because the regex requires .[a-zA-Z]{2,} at the end.
	input := "Contact user@example for help"
	got := f.Sanitize(input)
	if got != input {
		t.Errorf("Partial email without TLD should not be redacted: %q -> %q", input, got)
	}
}

func TestSanitizeEmailWithUnicodeDomain(t *testing.T) {
	f := New()
	// Standard ASCII email with IDN-like domain
	input := "reach user@xn--nxasmq6b.com for info"
	got := f.Sanitize(input)
	if strings.Contains(got, "user@") {
		t.Errorf("Email with IDN domain not redacted: %q", got)
	}
}

func TestSanitizePathWithSpacesAndUnicode(t *testing.T) {
	f := New()
	tests := []struct {
		name  string
		input string
		leak  string
	}{
		// Paths with spaces are terminated at the space by the regex,
		// but the username portion should still be caught
		{"path with unicode dir", "File at /Users/jean-luc/Documents/file.txt", "/Users/jean-luc"},
		{"home with dots", "Log: /home/user.name/.config/secret", "/home/user.name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Sanitize(tt.input)
			if strings.Contains(got, tt.leak) {
				t.Errorf("Path not redacted: %q", got)
			}
		})
	}
}

func TestSanitizeFalsePositivesExtended(t *testing.T) {
	f := New()
	// These should NOT be modified by the filter
	clean := []struct {
		name  string
		input string
	}{
		{"date format", "Created on 2026-03-04"},
		{"time range", "Open 09:00-17:00 daily"},
		{"commit hash short", "Fixed in commit abc1234"},
		{"UUID-like", "Session ID: a1b2c3d4"},
		{"go module version", "require github.com/pkg/errors v0.9.1"},
		{"semver", "Bumped to v1.23.45"},
		{"math expression", "Result: 3 + 5 = 8"},
		{"URL without IP", "Visit the docs at https://docs.example.com"},
		{"colon-separated non-hex", "Status: running: healthy"},
		{"percentage", "CPU usage: 85.5%"},
		{"currency", "Cost: $1,234.56"},
		{"vault entry count", "Vault has 12345 entries"},
		{"git branch name", "Merged feature/add-auth-module"},
		{"error with numbers", "Exit code 127: command not found"},
		{"config key", "max_connections = 100"},
	}
	for _, tt := range clean {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Sanitize(tt.input)
			if got != tt.input {
				t.Errorf("False positive: %q was changed to %q", tt.input, got)
			}
		})
	}
}

func TestBlocklistMalformedRegex(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	sameDir := filepath.Join(tmpDir, ".same")
	os.MkdirAll(sameDir, 0o755)

	// Mix valid and invalid patterns -- invalid should be silently skipped
	blocklist := "[invalid-regex\nvalid-pattern\\d+\n(another[bad\n"
	os.WriteFile(filepath.Join(sameDir, "blocklist.txt"), []byte(blocklist), 0o644)

	f := New()

	// Valid pattern should still work
	got := f.Sanitize("has valid-pattern42 here")
	if strings.Contains(got, "valid-pattern42") {
		t.Errorf("Valid blocklist pattern not applied despite malformed siblings: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("Expected [REDACTED], got: %q", got)
	}

	// Default patterns should still work too
	got2 := f.Sanitize("email user@example.com here")
	if strings.Contains(got2, "user@example.com") {
		t.Errorf("Default patterns broken by malformed blocklist: %q", got2)
	}
}

func TestBlocklistEmptyLines(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	sameDir := filepath.Join(tmpDir, ".same")
	os.MkdirAll(sameDir, 0o755)

	// Blocklist with only comments and empty lines
	blocklist := "# Just comments\n\n# Nothing useful\n   \n"
	os.WriteFile(filepath.Join(sameDir, "blocklist.txt"), []byte(blocklist), 0o644)

	f := New()
	// Should not panic and default patterns should work
	got := f.Sanitize("email user@example.com here")
	if strings.Contains(got, "user@example.com") {
		t.Errorf("Default patterns should work with empty blocklist: %q", got)
	}
}

func TestSanitizePerformanceLargeInput(t *testing.T) {
	f := New()
	// 100KB of text with PII scattered throughout
	var b strings.Builder
	for i := 0; i < 1000; i++ {
		b.WriteString("This is a normal line of text with no sensitive data.\n")
		if i%100 == 0 {
			b.WriteString("Contact admin@internal.corp for details.\n")
		}
		if i%200 == 0 {
			b.WriteString("Server at 10.0.0.1 is healthy.\n")
		}
	}
	input := b.String()

	// Just verify it completes without error and redacts
	got := f.Sanitize(input)
	if strings.Contains(got, "admin@internal.corp") {
		t.Error("Email in large input not redacted")
	}
	if strings.Contains(got, "10.0.0.1") {
		t.Error("IP in large input not redacted")
	}
}

// ---------------------------------------------------------------------------
// Telegram bot token security tests
// ---------------------------------------------------------------------------

func TestSanitizeTelegramBotToken(t *testing.T) {
	f := New()
	tests := []struct {
		name   string
		input  string
		redact bool
	}{
		// Valid bot token formats that MUST be caught
		{"standard token", "bot token: 1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw", true},
		{"token 8-digit id", "12345678:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw", true},
		{"token 10-digit id", "1234567890:ABCDEfghijKLMNOpqrstUVWXYz0123456789a", true},
		{"token with underscore", "1234567890:AA_bcdefghijklmnopqrstuvwxyz0123456", true},
		{"token with hyphen", "1234567890:AA-bcdefghijklmnopqrstuvwxyz0123456", true},
		{"token in config line", "bot_token = \"1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw\"", true},
		{"token in TOML", "token = \"9876543210:BBxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\"", true},
		{"token in JSON", `{"token":"1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"}`, true},
		{"token in URL", "https://api.telegram.org/bot1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw/getMe", true},
		{"token in env var", "TELEGRAM_BOT_TOKEN=1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw", true},
		{"token in export", "export BOT_TOKEN=1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw", true},
		{"token in log output", "Starting bot with token 1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw ...", true},

		// Should NOT be caught (false positives)
		{"short numeric:alpha", "12345:short", false},
		// Note: no-colon string is 44+ chars of alphanum, caught by base64 pattern (expected)
		{"no colon (caught by base64)", "1234567890AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw", true},
		{"7-digit id", "1234567:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Sanitize(tt.input)
			hasRedaction := strings.Contains(got, "[REDACTED]")
			if tt.redact && !hasRedaction {
				t.Errorf("Telegram token NOT redacted: input=%q got=%q", tt.input, got)
			}
			if !tt.redact && hasRedaction {
				t.Errorf("Unexpected redaction (false positive): input=%q got=%q", tt.input, got)
			}
		})
	}
}

func TestSanitizeTelegramBotTokenFullRedaction(t *testing.T) {
	f := New()
	// Verify the entire token is redacted, not just part of it
	token := "1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"
	input := "token is " + token + " end"
	got := f.Sanitize(input)

	if strings.Contains(got, "1234567890:") {
		t.Errorf("Token numeric prefix leaked: %q", got)
	}
	if strings.Contains(got, "AAHdqTcvCH") {
		t.Errorf("Token alphanumeric suffix leaked: %q", got)
	}
}

func TestSanitizeTelegramBotTokenInMultilineLogs(t *testing.T) {
	f := New()
	input := `2026-03-04 10:00:00 INFO Starting daemon
2026-03-04 10:00:01 DEBUG Token loaded: 1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw
2026-03-04 10:00:02 INFO Bot connected`

	got := f.Sanitize(input)
	if strings.Contains(got, "1234567890:AA") {
		t.Errorf("Token in multiline log not redacted:\n%s", got)
	}
	// Surrounding log lines should be preserved
	if !strings.Contains(got, "Starting daemon") {
		t.Error("Non-sensitive log line was corrupted")
	}
	if !strings.Contains(got, "Bot connected") {
		t.Error("Non-sensitive log line was corrupted")
	}
}

func TestSanitizeTelegramBotTokenInTOMLConfig(t *testing.T) {
	f := New()
	input := `[bot]
token = "1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"
allowed_user_ids = [95419682]

[notify]
on_hook = true`

	got := f.Sanitize(input)
	if strings.Contains(got, "1234567890:AA") {
		t.Errorf("Token in TOML config not redacted:\n%s", got)
	}
	// Structure keys should remain
	if !strings.Contains(got, "[bot]") {
		t.Error("[bot] section header corrupted")
	}
	if !strings.Contains(got, "[notify]") {
		t.Error("[notify] section header corrupted")
	}
}

func TestSanitizeTelegramUserID(t *testing.T) {
	f := New()
	tests := []struct {
		name   string
		input  string
		redact bool
	}{
		{"user_id in config", "user_id = 95419682", true},
		{"USER_ID uppercase", "USER_ID: 95419682", true},
		{"chat_id", "chat_id = 12345678", true},
		{"allowed_user with bracket", "allowed_user_ids = [95419682]", true},
		{"userId camelCase", "userId = 95419682", true},
		// Should not catch unrelated numbers
		{"plain number", "Port 8080 is open", false},
		{"version number", "Version 12345678", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Sanitize(tt.input)
			hasRedaction := strings.Contains(got, "[REDACTED]")
			if tt.redact && !hasRedaction {
				t.Errorf("Telegram user ID NOT redacted: input=%q got=%q", tt.input, got)
			}
			if !tt.redact && hasRedaction {
				t.Errorf("Unexpected redaction: input=%q got=%q", tt.input, got)
			}
		})
	}
}

func TestSanitizeTelegramTokenConcurrent(t *testing.T) {
	f := New()
	token := "1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"
	input := "Bot token: " + token

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := f.Sanitize(input)
			if strings.Contains(got, token) {
				t.Errorf("Token leaked in concurrent call: %q", got)
			}
		}()
	}
	wg.Wait()
}

func TestSanitizeTelegramTokenGenericSecretPattern(t *testing.T) {
	// The generic "token = value" pattern should also catch tokens
	// even if the specific telegram pattern somehow missed them
	f := New()
	tests := []struct {
		name  string
		input string
	}{
		{"token=", "token=1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"},
		{"bot_token:", "bot_token: 1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"},
		{"TOKEN =", "TOKEN = 1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Sanitize(tt.input)
			if !strings.Contains(got, "[REDACTED]") {
				t.Errorf("Generic secret pattern missed token: input=%q got=%q", tt.input, got)
			}
		})
	}
}

func TestSanitizeTelegramTokenInGitDiff(t *testing.T) {
	f := New()
	// Simulates a token accidentally showing up in git diff output
	input := `diff --git a/.same/telegram.toml b/.same/telegram.toml
+++ b/.same/telegram.toml
@@ -0,0 +1,5 @@
+[bot]
+token = "1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"
+allowed_user_ids = [95419682]`

	got := f.Sanitize(input)
	if strings.Contains(got, "1234567890:AA") {
		t.Error("Token in git diff output not redacted")
	}
}

func TestSanitizeTelegramTokenInErrorMessage(t *testing.T) {
	f := New()
	// Bot API error messages sometimes echo the token
	input := `telegram: unauthorized: bot token "1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw" is invalid`
	got := f.Sanitize(input)
	if strings.Contains(got, "1234567890:AA") {
		t.Error("Token in error message not redacted")
	}
}

func BenchmarkSanitizeTelegramToken(b *testing.B) {
	f := New()
	input := "Bot started with token 1234567890:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw successfully"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Sanitize(input)
	}
}

func BenchmarkSanitizeShort(b *testing.B) {
	f := New()
	input := "Contact user@example.com at 192.168.1.1 for details"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Sanitize(input)
	}
}

func BenchmarkSanitizeClean(b *testing.B) {
	f := New()
	input := "This is a perfectly normal message with no PII at all."
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Sanitize(input)
	}
}

func BenchmarkSanitizeLarge(b *testing.B) {
	f := New()
	input := strings.Repeat("Normal text without PII. ", 200)
	input += "leak@example.com"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Sanitize(input)
	}
}

func TestSanitizeMultipleRedactionsPreserveSurrounding(t *testing.T) {
	f := New()
	input := "From john@a.com to jane@b.com: meeting at /Users/john/notes.md"
	got := f.Sanitize(input)

	// The surrounding text structure should remain
	if !strings.Contains(got, "From") {
		t.Errorf("'From' prefix lost: %q", got)
	}
	if !strings.Contains(got, "to") {
		t.Errorf("'to' connector lost: %q", got)
	}
	if !strings.Contains(got, "meeting at") {
		t.Errorf("'meeting at' context lost: %q", got)
	}

	// But PII should be gone
	if strings.Contains(got, "john@") || strings.Contains(got, "jane@") {
		t.Errorf("Email leaked: %q", got)
	}
}

func TestSanitizeEmailEdgeCases(t *testing.T) {
	f := New()
	tests := []struct {
		name    string
		input   string
		redact  bool
	}{
		{"standard email", "hello user@example.com bye", true},
		{"email with plus", "hello user+tag@example.com bye", true},
		{"email with dots", "hello first.last@sub.example.co.uk bye", true},
		{"email with hyphens", "hello user@my-domain.org bye", true},
		{"no TLD (partial)", "hello user@localhost bye", false},
		{"at sign alone", "hello @ world", false},
		{"double at", "hello user@@example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Sanitize(tt.input)
			hasRedaction := strings.Contains(got, "[REDACTED]")
			if tt.redact && !hasRedaction {
				t.Errorf("Expected redaction for %q, got: %q", tt.input, got)
			}
			if !tt.redact && hasRedaction {
				t.Errorf("Unexpected redaction for %q, got: %q", tt.input, got)
			}
		})
	}
}

func TestSanitizeSSNEdgeCases(t *testing.T) {
	f := New()
	tests := []struct {
		name   string
		input  string
		redact bool
	}{
		{"standard SSN", "SSN: 123-45-6789", true},
		{"SSN in text", "number is 987-65-4321 here", true},
		{"too few digits", "code 12-34-5678", false},
		{"date format", "born 1990-12-25", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Sanitize(tt.input)
			hasRedaction := strings.Contains(got, "[REDACTED]")
			if tt.redact && !hasRedaction {
				t.Errorf("Expected SSN redaction for %q, got: %q", tt.input, got)
			}
			if !tt.redact && hasRedaction {
				t.Errorf("Unexpected SSN redaction for %q, got: %q", tt.input, got)
			}
		})
	}
}
