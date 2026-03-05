package msgbox

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func withTempHome(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	return tmpDir, func() { os.Setenv("HOME", origHome) }
}

func TestMessageJSON(t *testing.T) {
	m := Message{
		From:      "backend-dev",
		Type:      MsgTypeBlocker,
		Subject:   "Need API key rotation",
		Body:      "The current key expires tomorrow.",
		Timestamp: time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.From != "backend-dev" {
		t.Errorf("From = %q, want %q", decoded.From, "backend-dev")
	}
	if decoded.Subject != "Need API key rotation" {
		t.Errorf("Subject = %q, want %q", decoded.Subject, "Need API key rotation")
	}
	if decoded.Type != MsgTypeBlocker {
		t.Errorf("Type = %q, want %q", decoded.Type, MsgTypeBlocker)
	}
}

func TestMessageJSONOmitEmpty(t *testing.T) {
	m := Message{
		From:      "qa-engineer",
		Subject:   "Test results",
		Body:      "All pass",
		Timestamp: time.Now(),
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]any
	json.Unmarshal(data, &raw)
	// Verify all expected fields are present
	if _, ok := raw["from"]; !ok {
		t.Error("from should be present")
	}
	if _, ok := raw["subject"]; !ok {
		t.Error("subject should be present")
	}
}

func TestReplyJSON(t *testing.T) {
	r := Reply{
		To:        "backend-dev",
		InReplyTo: "msg-20260304.json",
		Body:      "Approved. Rotate the key.",
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Reply
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.To != "backend-dev" {
		t.Errorf("To = %q, want %q", decoded.To, "backend-dev")
	}
	if decoded.InReplyTo != "msg-20260304.json" {
		t.Errorf("InReplyTo = %q, want %q", decoded.InReplyTo, "msg-20260304.json")
	}
}

func TestEnsureDirs(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	err := EnsureDirs()
	if err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	for _, dir := range []string{OutboundDir(), InboundDir()} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("Directory %s not created: %v", dir, err)
		} else if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
	}
}

func TestEnsureDirsIdempotent(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	EnsureDirs()
	err := EnsureDirs()
	if err != nil {
		t.Fatalf("Second EnsureDirs should be idempotent: %v", err)
	}
}

func TestReadMessage(t *testing.T) {
	dir := t.TempDir()
	msg := Message{
		From:      "web-dev",
		Type:      MsgTypeStatus,
		Subject:   "Deploy approval needed",
		Body:      "Ready to ship v2.0",
		Timestamp: time.Now(),
	}
	data, _ := json.Marshal(msg)
	path := filepath.Join(dir, "test-msg.json")
	os.WriteFile(path, data, 0o644)

	got, err := ReadMessage(path)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if got.From != "web-dev" {
		t.Errorf("From = %q, want %q", got.From, "web-dev")
	}
	if got.Subject != "Deploy approval needed" {
		t.Errorf("Subject = %q", got.Subject)
	}
}

func TestReadMessageInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("not json {{{"), 0o644)

	_, err := ReadMessage(path)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestReadMessageMissingFile(t *testing.T) {
	_, err := ReadMessage("/nonexistent/path.json")
	if err == nil {
		t.Error("Expected error for missing file")
	}
}

func TestWriteReply(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	reply := &Reply{
		To:        "backend-dev",
		InReplyTo: "msg-123.json",
		Body:      "Go ahead with the migration.",
		Timestamp: time.Now(),
	}

	err := WriteReply(reply)
	if err != nil {
		t.Fatalf("WriteReply: %v", err)
	}

	// Verify file was written
	entries, err := os.ReadDir(InboundDir())
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Expected 1 reply file, got %d", len(entries))
	}

	name := entries[0].Name()
	if !strings.HasPrefix(name, "reply-") {
		t.Errorf("Reply filename should start with 'reply-', got: %s", name)
	}
	if !strings.HasSuffix(name, ".json") {
		t.Errorf("Reply filename should end with .json, got: %s", name)
	}

	// Read and verify content
	data, _ := os.ReadFile(filepath.Join(InboundDir(), name))
	var decoded Reply
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal reply: %v", err)
	}
	if decoded.To != "backend-dev" {
		t.Errorf("Reply.To = %q, want %q", decoded.To, "backend-dev")
	}
	if decoded.InReplyTo != "msg-123.json" {
		t.Errorf("Reply.InReplyTo = %q", decoded.InReplyTo)
	}
}

func TestArchiveMessage(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	EnsureDirs()

	// Write a message in the outbound dir
	path := filepath.Join(OutboundDir(), "msg.json")
	os.WriteFile(path, []byte(`{"from":"test"}`), 0o644)

	err := ArchiveMessage(path)
	if err != nil {
		t.Fatalf("ArchiveMessage: %v", err)
	}

	// Original should be gone
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("Original file should not exist after archive")
	}

	// File should be in the processed directory
	processed := filepath.Join(ProcessedDir(), "msg.json")
	if _, err := os.Stat(processed); err != nil {
		t.Errorf("Processed file should exist: %v", err)
	}
}

func TestWatcherScanOnce(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	EnsureDirs()

	// Write a test message
	msg := Message{
		From:      "qa-engineer",
		Subject:   "Tests passing",
		Body:      "All green",
		Timestamp: time.Now(),
	}
	data, _ := json.Marshal(msg)
	os.WriteFile(filepath.Join(OutboundDir(), "test-msg.json"), data, 0o644)

	// Set up handler
	received := make(chan *Message, 1)
	handler := func(m *Message, filename string) {
		received <- m
	}

	logger := log.New(os.Stderr, "test: ", 0)
	w := NewWatcher(logger, handler)
	w.ScanOnce()

	select {
	case m := <-received:
		if m.From != "qa-engineer" {
			t.Errorf("From = %q, want %q", m.From, "qa-engineer")
		}
		if m.Subject != "Tests passing" {
			t.Errorf("Subject = %q", m.Subject)
		}
	default:
		t.Error("Handler was not called")
	}

	// Verify message was archived (moved to processed dir)
	entries, _ := os.ReadDir(OutboundDir())
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			t.Errorf("Unprocessed message still in outbound: %s", e.Name())
		}
	}
}

func TestWatcherSkipsNonJSON(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	EnsureDirs()

	// Write non-JSON files that should be skipped
	os.WriteFile(filepath.Join(OutboundDir(), "readme.txt"), []byte("not a message"), 0o644)
	os.WriteFile(filepath.Join(OutboundDir(), "notes.md"), []byte("# Notes"), 0o644)

	called := false
	handler := func(m *Message, filename string) {
		called = true
	}

	logger := log.New(os.Stderr, "test: ", 0)
	w := NewWatcher(logger, handler)
	w.ScanOnce()

	if called {
		t.Error("Handler should not be called for non-JSON files")
	}
}

func TestWatcherSkipsProcessed(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	EnsureDirs()

	// Write an already-processed file
	msg := Message{From: "test", Subject: "old", Body: "done", Timestamp: time.Now()}
	data, _ := json.Marshal(msg)
	os.WriteFile(filepath.Join(OutboundDir(), "old.json.processed"), data, 0o644)

	called := false
	handler := func(m *Message, filename string) {
		called = true
	}

	logger := log.New(os.Stderr, "test: ", 0)
	w := NewWatcher(logger, handler)
	w.ScanOnce()

	if called {
		t.Error("Handler should not be called for .processed files")
	}
}

func TestWatcherSkipsDirectories(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	EnsureDirs()
	os.Mkdir(filepath.Join(OutboundDir(), "subdir"), 0o755)

	called := false
	handler := func(m *Message, filename string) {
		called = true
	}

	logger := log.New(os.Stderr, "test: ", 0)
	w := NewWatcher(logger, handler)
	w.ScanOnce()

	if called {
		t.Error("Handler should not be called for directories")
	}
}

func TestWatcherInvalidJSONArchived(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	EnsureDirs()

	// Write invalid JSON
	path := filepath.Join(OutboundDir(), "bad.json")
	os.WriteFile(path, []byte("not valid json"), 0o644)

	called := false
	handler := func(m *Message, filename string) {
		called = true
	}

	logger := log.New(os.Stderr, "test: ", 0)
	w := NewWatcher(logger, handler)
	w.ScanOnce()

	if called {
		t.Error("Handler should not be called for invalid JSON")
	}

	// Invalid file should be archived to processed dir (not left to block queue)
	processed := filepath.Join(ProcessedDir(), "bad.json")
	if _, err := os.Stat(processed); os.IsNotExist(err) {
		t.Error("Invalid JSON file should be archived to prevent blocking")
	}
}

func TestWatcherMultipleMessages(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	EnsureDirs()

	for i := 0; i < 3; i++ {
		msg := Message{
			From:      "agent",
			Subject:   "msg",
			Body:      "body",
			Timestamp: time.Now(),
		}
		data, _ := json.Marshal(msg)
		name := filepath.Join(OutboundDir(), strings.Replace(time.Now().Format("20060102-150405.000"), ".", "", 1)+".json")
		// Ensure unique names
		os.WriteFile(name+string(rune('a'+i)), data, 0o644)
	}

	// Rewrite with .json extension
	entries, _ := os.ReadDir(OutboundDir())
	count := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			count++
		}
	}

	// Just write 3 unique files directly
	for _, name := range []string{"a.json", "b.json", "c.json"} {
		msg := Message{From: "agent", Subject: name, Body: "body", Timestamp: time.Now()}
		data, _ := json.Marshal(msg)
		os.WriteFile(filepath.Join(OutboundDir(), name), data, 0o644)
	}

	received := 0
	handler := func(m *Message, filename string) {
		received++
	}

	logger := log.New(os.Stderr, "test: ", 0)
	w := NewWatcher(logger, handler)
	w.ScanOnce()

	if received < 3 {
		t.Errorf("Expected at least 3 messages processed, got %d", received)
	}
}

func TestWatcherContextCancellation(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	EnsureDirs()

	handler := func(m *Message, filename string) {}
	logger := log.New(os.Stderr, "test: ", 0)
	w := NewWatcher(logger, handler)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Watch(ctx)
		close(done)
	}()

	// Cancel after a short time
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Watch returned, good
	case <-time.After(3 * time.Second):
		t.Error("Watch did not return after context cancellation")
	}
}

func TestOutboundInboundDirPaths(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	outbound := OutboundDir()
	inbound := InboundDir()

	if !strings.Contains(outbound, "messages/outbound") {
		t.Errorf("OutboundDir = %q, expected to contain messages/outbound", outbound)
	}
	if !strings.Contains(inbound, "messages/inbound") {
		t.Errorf("InboundDir = %q, expected to contain messages/inbound", inbound)
	}
	if !strings.Contains(outbound, "company-hq") {
		t.Errorf("OutboundDir = %q, expected to be under company-hq", outbound)
	}
}

func TestWriteMessage(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	msg := &Message{
		From:    "backend-dev",
		Type:    MsgTypeQuestion,
		Subject: "Need approval",
		Body:    "Can we proceed?",
	}

	filename, err := WriteMessage(msg)
	if err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	if !strings.HasPrefix(filename, "backend-dev-") {
		t.Errorf("Filename should start with agent name, got: %s", filename)
	}
	if !strings.HasSuffix(filename, ".json") {
		t.Errorf("Filename should end with .json, got: %s", filename)
	}

	// Verify timestamp was set
	if msg.Timestamp.IsZero() {
		t.Error("WriteMessage should set timestamp when zero")
	}

	// Verify file content
	path := filepath.Join(OutboundDir(), filename)
	got, err := ReadMessage(path)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if got.From != "backend-dev" {
		t.Errorf("From = %q, want backend-dev", got.From)
	}
	if got.Type != MsgTypeQuestion {
		t.Errorf("Type = %q, want %q", got.Type, MsgTypeQuestion)
	}
}

func TestWriteMessagePreservesTimestamp(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	ts := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	msg := &Message{
		From:      "agent",
		Body:      "test",
		Timestamp: ts,
	}

	_, err := WriteMessage(msg)
	if err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	if !msg.Timestamp.Equal(ts) {
		t.Errorf("WriteMessage should preserve non-zero timestamp, got %v", msg.Timestamp)
	}
}

func TestProcessedDir(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	pd := ProcessedDir()
	if !strings.Contains(pd, "outbound/processed") {
		t.Errorf("ProcessedDir = %q, expected to contain outbound/processed", pd)
	}
}

func TestReadMessageEmptyBody(t *testing.T) {
	dir := t.TempDir()
	msg := Message{
		From:      "agent",
		Subject:   "Empty body test",
		Body:      "",
		Timestamp: time.Now(),
	}
	data, _ := json.Marshal(msg)
	path := filepath.Join(dir, "empty-body.json")
	os.WriteFile(path, data, 0o644)

	got, err := ReadMessage(path)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if got.Body != "" {
		t.Errorf("Body should be empty, got %q", got.Body)
	}
}
