package bot

import (
	"strings"
	"testing"
)

func TestClaudeToggleDefault(t *testing.T) {
	ct := newClaudeToggle()
	if ct.isEnabled(123) {
		t.Error("Claude mode should be disabled by default")
	}
}

func TestClaudeToggleOnOff(t *testing.T) {
	ct := newClaudeToggle()

	ct.set(123, true)
	if !ct.isEnabled(123) {
		t.Error("Expected Claude mode enabled after set(true)")
	}

	ct.set(123, false)
	if ct.isEnabled(123) {
		t.Error("Expected Claude mode disabled after set(false)")
	}
}

func TestClaudeTogglePerUser(t *testing.T) {
	ct := newClaudeToggle()

	ct.set(100, true)
	ct.set(200, false)

	if !ct.isEnabled(100) {
		t.Error("User 100 should have Claude enabled")
	}
	if ct.isEnabled(200) {
		t.Error("User 200 should have Claude disabled")
	}
	if ct.isEnabled(300) {
		t.Error("Unknown user 300 should default to disabled")
	}
}

func TestChunkTextShort(t *testing.T) {
	text := "Hello world"
	chunks := chunkText(text, 100)
	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("Chunk = %q, want %q", chunks[0], text)
	}
}

func TestChunkTextExactLimit(t *testing.T) {
	text := strings.Repeat("a", 100)
	chunks := chunkText(text, 100)
	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk for text at exact limit, got %d", len(chunks))
	}
}

func TestChunkTextLong(t *testing.T) {
	// 250 chars, max 100 per chunk
	text := strings.Repeat("a", 250)
	chunks := chunkText(text, 100)
	if len(chunks) != 3 {
		t.Fatalf("Expected 3 chunks, got %d", len(chunks))
	}
	// Reconstruct should equal original
	reconstructed := strings.Join(chunks, "")
	if reconstructed != text {
		t.Error("Chunks should reconstruct to original text")
	}
}

func TestChunkTextBreaksAtNewline(t *testing.T) {
	text := "Line one here\nLine two here\nLine three is longer text that keeps going"
	chunks := chunkText(text, 30)

	// First chunk should break at newline, not mid-word
	if len(chunks) < 2 {
		t.Fatalf("Expected multiple chunks, got %d", len(chunks))
	}
	if !strings.HasSuffix(chunks[0], "\n") {
		t.Errorf("First chunk should break at newline: %q", chunks[0])
	}
}

func TestChunkTextEmpty(t *testing.T) {
	chunks := chunkText("", 100)
	if len(chunks) != 1 || chunks[0] != "" {
		t.Errorf("Empty text should produce single empty chunk, got %v", chunks)
	}
}

func TestChunkTextNoNewlines(t *testing.T) {
	text := strings.Repeat("x", 300)
	chunks := chunkText(text, 100)
	if len(chunks) != 3 {
		t.Fatalf("Expected 3 chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if i < len(chunks)-1 && len(c) != 100 {
			t.Errorf("Chunk %d should be 100 chars, got %d", i, len(c))
		}
	}
}

func TestHelpTextIncludesAICommands(t *testing.T) {
	text := helpText()
	// The /ai command is listed in the registry; sub-commands are arguments.
	for _, cmd := range []string{"/ai", "/claude", "/reset", "/onboard"} {
		if !strings.Contains(text, cmd) {
			t.Errorf("helpText missing %s", cmd)
		}
	}
}

func TestClaudeToggleConcurrency(t *testing.T) {
	ct := newClaudeToggle()
	done := make(chan struct{})

	// Concurrent writes and reads
	for i := 0; i < 50; i++ {
		go func(id int64) {
			ct.set(id, true)
			_ = ct.isEnabled(id)
			ct.set(id, false)
			done <- struct{}{}
		}(int64(i))
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}

func TestClaudeAIModePriority(t *testing.T) {
	// AI mode takes priority over Claude mode (matches bot.go handleUpdate logic)
	claude := newClaudeToggle()
	ai := newAIState()

	userID := int64(42)

	claude.set(userID, true)
	ai.setEnabled(userID, true)

	// AI should take priority
	if !ai.isEnabled(userID) {
		t.Error("AI mode should be enabled")
	}
	if !claude.isEnabled(userID) {
		t.Error("Claude mode should also be enabled (but AI takes priority in dispatch)")
	}

	// When AI is off, Claude should still work
	ai.setEnabled(userID, false)
	if !claude.isEnabled(userID) {
		t.Error("Claude mode should remain enabled when AI is turned off")
	}
}

func TestChunkTextMaxTelegramMessage(t *testing.T) {
	// Verify the constant is correct
	if maxTelegramMessage != 4096 {
		t.Errorf("maxTelegramMessage should be 4096, got %d", maxTelegramMessage)
	}

	// Verify chunking with the actual margin used in handleClaudeMessage
	margin := maxTelegramMessage - 100
	text := strings.Repeat("a", maxTelegramMessage*2)
	chunks := chunkText(text, margin)

	for i, chunk := range chunks {
		if len(chunk) > margin {
			t.Errorf("chunk %d exceeds margin: %d > %d", i, len(chunk), margin)
		}
	}

	reconstructed := strings.Join(chunks, "")
	if reconstructed != text {
		t.Error("chunks should reconstruct to original text")
	}
}
