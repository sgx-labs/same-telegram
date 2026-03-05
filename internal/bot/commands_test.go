package bot

import (
	"strings"
	"sync"
	"testing"

	"github.com/sgx-labs/same-telegram/internal/config"
)

func TestHelpText(t *testing.T) {
	text := helpText()
	if text == "" {
		t.Error("helpText should not be empty")
	}
	// Verify all registered commands are present in help text
	commands := []string{
		"/status", "/doctor", "/search", "/ask",
		"/help", "/team", "/decisions", "/announce",
		"/claude", "/ai", "/reset", "/onboard", "/settings",
		"/reviews", "/review", "/approve", "/reject",
		"/task", "/tasks", "/cancel-task",
		"/stop", "/usage",
	}
	for _, cmd := range commands {
		if !strings.Contains(text, cmd) {
			t.Errorf("helpText missing command: %s", cmd)
		}
	}
}

func TestHelpTextMarkdown(t *testing.T) {
	text := helpText()
	// Should have bold markers for section headers
	if !strings.Contains(text, "*") {
		t.Error("helpText should contain Markdown bold markers")
	}
}

func TestCmdSearchEmpty(t *testing.T) {
	reply, err := cmdSearch("")
	if err != nil {
		t.Fatalf("cmdSearch empty: %v", err)
	}
	if !strings.Contains(reply, "Usage:") {
		t.Errorf("Empty search should show usage, got: %s", reply)
	}
}

func TestCmdSearchWhitespace(t *testing.T) {
	reply, err := cmdSearch("   ")
	if err != nil {
		t.Fatalf("cmdSearch whitespace: %v", err)
	}
	if !strings.Contains(reply, "Usage:") {
		t.Errorf("Whitespace search should show usage, got: %s", reply)
	}
}

func TestCmdAskEmpty(t *testing.T) {
	reply, err := cmdAsk("")
	if err != nil {
		t.Fatalf("cmdAsk empty: %v", err)
	}
	if !strings.Contains(reply, "Usage:") {
		t.Errorf("Empty ask should show usage, got: %s", reply)
	}
}

func TestCmdAskWhitespace(t *testing.T) {
	reply, err := cmdAsk("   ")
	if err != nil {
		t.Fatalf("cmdAsk whitespace: %v", err)
	}
	if !strings.Contains(reply, "Usage:") {
		t.Errorf("Whitespace ask should show usage, got: %s", reply)
	}
}

func TestCmdConfig(t *testing.T) {
	cfg := &config.Config{
		Bot: config.BotConfig{
			Token:          "test-token",
			AllowedUserIDs: []int64{111, 222},
		},
		Notify: config.NotifyConfig{
			SessionEnd: true,
			Decisions:  false,
			Handoffs:   true,
		},
		Digest: config.DigestConfig{
			Enabled: true,
			Time:    "09:00",
		},
	}

	b := &Bot{cfg: cfg}
	text := cmdConfig(b)

	if !strings.Contains(text, "Current Configuration") {
		t.Errorf("Expected 'Current Configuration' header, got: %s", text)
	}
	if !strings.Contains(text, "true") {
		t.Error("Expected 'true' for session_end")
	}
	if !strings.Contains(text, "false") {
		t.Error("Expected 'false' for decisions")
	}
	if !strings.Contains(text, "09:00") {
		t.Error("Expected digest time '09:00'")
	}
	if !strings.Contains(text, "2 configured") {
		t.Errorf("Expected '2 configured' for allowed users, got: %s", text)
	}
}

func TestCmdConfigZeroUsers(t *testing.T) {
	cfg := &config.Config{
		Bot:    config.BotConfig{AllowedUserIDs: []int64{}},
		Notify: config.NotifyConfig{},
		Digest: config.DigestConfig{},
	}
	b := &Bot{cfg: cfg}
	text := cmdConfig(b)
	if !strings.Contains(text, "0 configured") {
		t.Errorf("Expected '0 configured', got: %s", text)
	}
}

func TestReplyTrackerConcurrent(t *testing.T) {
	rt := newReplyTracker()
	var wg sync.WaitGroup

	// Concurrent tracks
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rt.track(id, "file.json", "agent")
		}(i)
	}
	wg.Wait()

	// Concurrent pops
	found := 0
	var mu sync.Mutex
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if _, ok := rt.pop(id); ok {
				mu.Lock()
				found++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if found != 100 {
		t.Errorf("Expected 100 pops, got %d", found)
	}
}

func TestReplyTrackerOverwrite(t *testing.T) {
	rt := newReplyTracker()
	rt.track(1, "first.json", "agent-a")
	rt.track(1, "second.json", "agent-b") // overwrite same ID

	pr, ok := rt.pop(1)
	if !ok {
		t.Fatal("Expected to find tracked reply")
	}
	// Should have the latest value
	if pr.filename != "second.json" {
		t.Errorf("filename = %q, want second.json", pr.filename)
	}
	if pr.agent != "agent-b" {
		t.Errorf("agent = %q, want agent-b", pr.agent)
	}
}
