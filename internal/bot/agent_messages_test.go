package bot

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sgx-labs/same-telegram/internal/msgbox"
)

func TestFormatAgentMessageQuestion(t *testing.T) {
	msg := &msgbox.Message{
		From:      "backend-dev",
		Type:      msgbox.MsgTypeQuestion,
		Subject:   "Need database approval",
		Body:      "Can we migrate to PostgreSQL?",
		Timestamp: time.Now(),
	}

	text := formatAgentMessage(msg)
	if !strings.Contains(text, "Question from backend-dev") {
		t.Errorf("Expected 'Question from backend-dev' in output, got: %s", text)
	}
	if !strings.Contains(text, "Need database approval") {
		t.Errorf("Expected subject in output, got: %s", text)
	}
	if !strings.Contains(text, "Can we migrate to PostgreSQL?") {
		t.Errorf("Expected body in output, got: %s", text)
	}
	if !strings.Contains(text, "Reply to this message") {
		t.Errorf("Expected reply hint in output, got: %s", text)
	}
}

func TestFormatAgentMessageBlocker(t *testing.T) {
	msg := &msgbox.Message{
		From:    "qa-engineer",
		Type:    msgbox.MsgTypeBlocker,
		Subject: "Critical bug found",
		Body:    "Production is down",
	}

	text := formatAgentMessage(msg)
	if !strings.Contains(text, "Blocker") {
		t.Errorf("Blocker type should show 'Blocker' label, got: %s", text)
	}
}

func TestFormatAgentMessageStatus(t *testing.T) {
	msg := &msgbox.Message{
		From:    "backend-dev",
		Type:    msgbox.MsgTypeStatus,
		Subject: "Sprint update",
		Body:    "All tasks complete",
	}

	text := formatAgentMessage(msg)
	if !strings.Contains(text, "Status Update") {
		t.Errorf("Status type should show 'Status Update' label, got: %s", text)
	}
}

func TestFormatAgentMessageUnknownType(t *testing.T) {
	msg := &msgbox.Message{
		From:    "agent",
		Type:    "custom",
		Subject: "Test",
		Body:    "body",
	}

	text := formatAgentMessage(msg)
	if !strings.Contains(text, "Message from agent") {
		t.Errorf("Unknown type should fall back to 'Message', got: %s", text)
	}
}

func TestFormatAgentMessageNoSubject(t *testing.T) {
	msg := &msgbox.Message{
		From: "agent",
		Body: "Quick question",
	}

	text := formatAgentMessage(msg)
	if strings.Contains(text, "Subject:") {
		t.Error("No Subject line expected when subject is empty")
	}
	if !strings.Contains(text, "Quick question") {
		t.Errorf("Body should appear in output: %s", text)
	}
}

func TestFormatAgentMessageEscapesMarkdown(t *testing.T) {
	msg := &msgbox.Message{
		From:    "agent_with_underscores",
		Subject: "Use *bold* in subject",
		Body:    "Some _italic_ text",
	}

	text := formatAgentMessage(msg)
	// Markdown special chars should be escaped
	if strings.Contains(text, "*bold*") && !strings.Contains(text, "\\*bold\\*") {
		t.Error("Subject markdown not escaped")
	}
}

func TestReplyTrackerTrackAndPop(t *testing.T) {
	rt := newReplyTracker()

	rt.track(42, "msg-001.json", "backend-dev")

	pr, ok := rt.pop(42)
	if !ok {
		t.Fatal("Expected to find tracked reply")
	}
	if pr.filename != "msg-001.json" {
		t.Errorf("filename = %q, want %q", pr.filename, "msg-001.json")
	}
	if pr.agent != "backend-dev" {
		t.Errorf("agent = %q, want %q", pr.agent, "backend-dev")
	}

	// Second pop should fail
	_, ok = rt.pop(42)
	if ok {
		t.Error("Second pop should return false (already consumed)")
	}
}

func TestReplyTrackerMiss(t *testing.T) {
	rt := newReplyTracker()

	_, ok := rt.pop(999)
	if ok {
		t.Error("Pop on empty tracker should return false")
	}
}

func TestReplyTrackerMultiple(t *testing.T) {
	rt := newReplyTracker()

	rt.track(1, "a.json", "agent-a")
	rt.track(2, "b.json", "agent-b")
	rt.track(3, "c.json", "agent-c")

	pr, ok := rt.pop(2)
	if !ok || pr.agent != "agent-b" {
		t.Errorf("Expected agent-b, got %+v", pr)
	}

	// Others should still be available
	pr, ok = rt.pop(1)
	if !ok || pr.agent != "agent-a" {
		t.Errorf("Expected agent-a, got %+v", pr)
	}

	pr, ok = rt.pop(3)
	if !ok || pr.agent != "agent-c" {
		t.Errorf("Expected agent-c, got %+v", pr)
	}
}

func TestFormatAgentMessageEmptyBodyAndSubject(t *testing.T) {
	msg := &msgbox.Message{
		From: "agent",
	}
	text := formatAgentMessage(msg)
	if !strings.Contains(text, "agent") {
		t.Errorf("Expected agent name in output: %s", text)
	}
	if strings.Contains(text, "Subject:") {
		t.Error("No Subject line expected when subject is empty")
	}
	// Should still have the reply hint
	if !strings.Contains(text, "Reply to this message") {
		t.Errorf("Expected reply hint: %s", text)
	}
}

func TestFormatAgentMessageAllTypes(t *testing.T) {
	tests := []struct {
		msgType string
		label   string
	}{
		{msgbox.MsgTypeQuestion, "Question"},
		{msgbox.MsgTypeStatus, "Status Update"},
		{msgbox.MsgTypeBlocker, "Blocker"},
		{"", "Message"},
		{"unknown-type", "Message"},
	}

	for _, tt := range tests {
		t.Run(tt.msgType, func(t *testing.T) {
			msg := &msgbox.Message{
				From: "test-agent",
				Type: tt.msgType,
				Body: "test body",
			}
			text := formatAgentMessage(msg)
			if !strings.Contains(text, tt.label) {
				t.Errorf("Type %q should produce label %q, got: %s", tt.msgType, tt.label, text)
			}
		})
	}
}

func TestFormatAgentMessageLongBody(t *testing.T) {
	longBody := strings.Repeat("A", 4096)
	msg := &msgbox.Message{
		From:    "agent",
		Subject: "Long message",
		Body:    longBody,
	}
	text := formatAgentMessage(msg)
	if !strings.Contains(text, longBody[:100]) {
		t.Error("Long body should be preserved in formatted output")
	}
}

func TestFormatAgentMessageSpecialCharsInFrom(t *testing.T) {
	msg := &msgbox.Message{
		From:    "agent_name.with.dots",
		Subject: "Test [brackets] and (parens)",
		Body:    "Body with `backticks` and |pipes|",
	}
	text := formatAgentMessage(msg)
	// Should contain escaped version of agent name with underscores
	if !strings.Contains(text, "agent") {
		t.Errorf("Expected agent name in output: %s", text)
	}
}

func TestReplyTrackerOverwriteSameID(t *testing.T) {
	rt := newReplyTracker()
	rt.track(1, "first.json", "agent-a")
	rt.track(1, "second.json", "agent-b")

	pr, ok := rt.pop(1)
	if !ok {
		t.Fatal("Expected to find tracked reply")
	}
	if pr.filename != "second.json" {
		t.Errorf("filename = %q, want second.json", pr.filename)
	}
	if pr.agent != "agent-b" {
		t.Errorf("agent = %q, want agent-b", pr.agent)
	}
}

func TestReplyTrackerConcurrentTrackPop(t *testing.T) {
	rt := newReplyTracker()
	var wg sync.WaitGroup

	// Concurrent tracks
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rt.track(id, fmt.Sprintf("msg-%d.json", id), fmt.Sprintf("agent-%d", id))
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
