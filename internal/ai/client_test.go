package ai

import (
	"testing"
)

func TestDefaultModel(t *testing.T) {
	tests := []struct {
		backend Backend
		want    string
	}{
		{BackendClaude, "claude-sonnet-4-20250514"},
		{BackendOpenAI, "gpt-4o"},
		{BackendGemini, "gemini-2.5-flash"},
		{BackendOllama, "qwen2.5-coder:7b"},
		{Backend("unknown"), ""},
	}

	for _, tt := range tests {
		got := DefaultModel(tt.backend)
		if got != tt.want {
			t.Errorf("DefaultModel(%q) = %q, want %q", tt.backend, got, tt.want)
		}
	}
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		backend Backend
		wantErr bool
	}{
		{BackendClaude, false},
		{BackendOpenAI, false},
		{BackendGemini, false},
		{BackendOllama, false},
		{Backend("unknown"), true},
	}

	for _, tt := range tests {
		c, err := NewClient(tt.backend, "test-key")
		if tt.wantErr {
			if err == nil {
				t.Errorf("NewClient(%q): expected error", tt.backend)
			}
			continue
		}
		if err != nil {
			t.Errorf("NewClient(%q): %v", tt.backend, err)
			continue
		}
		if c == nil {
			t.Errorf("NewClient(%q): client is nil", tt.backend)
		}
	}
}

func TestTruncateError(t *testing.T) {
	short := []byte("short error")
	if got := truncateError(short); got != "short error" {
		t.Errorf("truncateError(short) = %q, want %q", got, "short error")
	}

	long := make([]byte, 300)
	for i := range long {
		long[i] = 'x'
	}
	got := truncateError(long)
	if len(got) > 210 {
		t.Errorf("truncateError(long) len = %d, want <= 210", len(got))
	}
}
