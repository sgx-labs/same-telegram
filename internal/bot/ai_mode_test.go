package bot

import (
	"strings"
	"testing"
)

func TestAIStateToggle(t *testing.T) {
	s := newAIState()

	if s.isEnabled(123) {
		t.Error("AI mode should be disabled by default")
	}

	s.setEnabled(123, true)
	if !s.isEnabled(123) {
		t.Error("AI mode should be enabled after setEnabled(true)")
	}

	s.setEnabled(123, false)
	if s.isEnabled(123) {
		t.Error("AI mode should be disabled after setEnabled(false)")
	}
}

func TestAIStateDefaultBackend(t *testing.T) {
	s := newAIState()
	backend := s.getBackend(123)
	if backend != "claude" {
		t.Errorf("Default backend should be 'claude', got %q", backend)
	}
}

func TestAIStateBackendSwitch(t *testing.T) {
	s := newAIState()

	s.setBackend(123, "codex")
	if b := s.getBackend(123); b != "codex" {
		t.Errorf("Backend should be 'codex', got %q", b)
	}

	s.setBackend(123, "gemini")
	if b := s.getBackend(123); b != "gemini" {
		t.Errorf("Backend should be 'gemini', got %q", b)
	}

	s.setBackend(123, "claude")
	if b := s.getBackend(123); b != "claude" {
		t.Errorf("Backend should be 'claude', got %q", b)
	}
}

func TestAIStatePerUser(t *testing.T) {
	s := newAIState()

	s.setEnabled(1, true)
	s.setBackend(1, "codex")

	s.setEnabled(2, true)
	s.setBackend(2, "gemini")

	if b := s.getBackend(1); b != "codex" {
		t.Errorf("User 1 backend should be 'codex', got %q", b)
	}
	if b := s.getBackend(2); b != "gemini" {
		t.Errorf("User 2 backend should be 'gemini', got %q", b)
	}

	// User 3 should have defaults
	if s.isEnabled(3) {
		t.Error("User 3 should not be enabled")
	}
	if b := s.getBackend(3); b != "claude" {
		t.Errorf("User 3 default backend should be 'claude', got %q", b)
	}
}

func TestAIBackendConfigs(t *testing.T) {
	expected := []string{"claude", "codex", "gemini"}
	for _, name := range expected {
		cfg, ok := aiBackends[name]
		if !ok {
			t.Errorf("Missing backend config for %q", name)
			continue
		}
		if cfg.Command == "" {
			t.Errorf("Backend %q has empty command", name)
		}
		if len(cfg.Args) == 0 {
			t.Errorf("Backend %q has no args", name)
		}
		// All backends should have -p as the last arg (prompt flag)
		lastArg := cfg.Args[len(cfg.Args)-1]
		if lastArg != "-p" {
			t.Errorf("Backend %q last arg should be '-p', got %q", name, lastArg)
		}
	}
}

func TestAIChunkTextShort(t *testing.T) {
	text := "Hello, world!"
	chunks := chunkText(text, 100)
	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("Chunk should match input, got %q", chunks[0])
	}
}

func TestAIChunkTextLong(t *testing.T) {
	// Create a string longer than the max
	text := strings.Repeat("Hello\n", 1000)
	maxLen := 100
	chunks := chunkText(text, maxLen)

	if len(chunks) < 2 {
		t.Errorf("Expected multiple chunks, got %d", len(chunks))
	}

	// Verify no chunk exceeds maxLen
	for i, chunk := range chunks {
		if len(chunk) > maxLen {
			t.Errorf("Chunk %d exceeds maxLen: %d > %d", i, len(chunk), maxLen)
		}
	}

	// Verify all text is preserved
	reassembled := strings.Join(chunks, "")
	if reassembled != text {
		t.Error("Reassembled chunks should equal original text")
	}
}

func TestAIChunkTextNoNewlines(t *testing.T) {
	// Text with no newlines should still be chunked
	text := strings.Repeat("x", 500)
	chunks := chunkText(text, 100)

	if len(chunks) != 5 {
		t.Errorf("Expected 5 chunks, got %d", len(chunks))
	}

	reassembled := strings.Join(chunks, "")
	if reassembled != text {
		t.Error("Reassembled chunks should equal original text")
	}
}

func TestHelpTextIncludesAI(t *testing.T) {
	text := helpText()
	commands := []string{"/ai"}
	for _, cmd := range commands {
		if !strings.Contains(text, cmd) {
			t.Errorf("helpText missing command: %s", cmd)
		}
	}
}
