package bot

import (
	"strings"
	"testing"

	"github.com/sgx-labs/same-telegram/internal/notify"
)

func TestFormatSessionEnd(t *testing.T) {
	n := &notify.Notification{
		Type:    notify.TypeSessionEnd,
		Summary: "Implemented auth module",
	}
	text, kb := FormatNotification(n)
	if !strings.Contains(text, "Session Ended") {
		t.Errorf("Expected 'Session Ended' in text, got: %s", text)
	}
	if kb != nil {
		t.Error("Session end should not have keyboard")
	}
}

func TestFormatDecision(t *testing.T) {
	n := &notify.Notification{
		Type:      notify.TypeDecision,
		SessionID: "abc-123",
		Summary:   "Use PostgreSQL for user data",
	}
	text, kb := FormatNotification(n)
	if !strings.Contains(text, "Decision Logged") {
		t.Errorf("Expected 'Decision Logged' in text, got: %s", text)
	}
	if kb == nil {
		t.Error("Decision should have inline keyboard")
	}
}

func TestEscapeMarkdown(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"hello", "hello"},
		{"hello_world", "hello\\_world"},
		{"*bold*", "\\*bold\\*"},
		{"[link]", "\\[link\\]"},
	}
	for _, tt := range tests {
		got := escapeMarkdown(tt.input)
		if got != tt.expected {
			t.Errorf("escapeMarkdown(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
