package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestHookInputJSON(t *testing.T) {
	input := HookInput{
		Prompt:         "do something",
		TranscriptPath: "/tmp/transcript.jsonl",
		SessionID:      "sess-abc-123",
		HookEventName:  "Stop",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded HookInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.HookEventName != "Stop" {
		t.Errorf("HookEventName = %q, want Stop", decoded.HookEventName)
	}
	if decoded.SessionID != "sess-abc-123" {
		t.Errorf("SessionID = %q, want sess-abc-123", decoded.SessionID)
	}
	if decoded.TranscriptPath != "/tmp/transcript.jsonl" {
		t.Errorf("TranscriptPath = %q", decoded.TranscriptPath)
	}
}

func TestHookInputOmitEmpty(t *testing.T) {
	input := HookInput{HookEventName: "Stop"}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var raw map[string]any
	json.Unmarshal(data, &raw)
	if _, ok := raw["prompt"]; ok {
		t.Error("prompt should be omitted when empty")
	}
	if _, ok := raw["transcript_path"]; ok {
		t.Error("transcript_path should be omitted when empty")
	}
	if _, ok := raw["session_id"]; ok {
		t.Error("session_id should be omitted when empty")
	}
}

func TestHookOutputJSON(t *testing.T) {
	output := HookOutput{SystemMessage: "hello"}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded HookOutput
	json.Unmarshal(data, &decoded)
	if decoded.SystemMessage != "hello" {
		t.Errorf("SystemMessage = %q, want hello", decoded.SystemMessage)
	}
}

func TestHookOutputOmitEmpty(t *testing.T) {
	output := HookOutput{}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var raw map[string]any
	json.Unmarshal(data, &raw)
	if _, ok := raw["systemMessage"]; ok {
		t.Error("systemMessage should be omitted when empty")
	}
}

func TestWriteHookOutput(t *testing.T) {
	// Capture stdout
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := writeHookOutput("", nil)

	w.Close()
	os.Stdout = origStdout

	if err != nil {
		t.Fatalf("writeHookOutput: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var decoded HookOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &decoded); err != nil {
		t.Fatalf("Unmarshal output: %v (raw: %q)", err, output)
	}
	if decoded.SystemMessage != "" {
		t.Errorf("Expected empty systemMessage, got %q", decoded.SystemMessage)
	}
}

func TestWriteHookOutputWithMessage(t *testing.T) {
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := writeHookOutput("test message", nil)

	w.Close()
	os.Stdout = origStdout

	if err != nil {
		t.Fatalf("writeHookOutput: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var decoded HookOutput
	json.Unmarshal([]byte(strings.TrimSpace(output)), &decoded)
	if decoded.SystemMessage != "test message" {
		t.Errorf("SystemMessage = %q, want 'test message'", decoded.SystemMessage)
	}
}

func TestWriteHookOutputWithError(t *testing.T) {
	// writeHookOutput should still produce valid JSON even when there's an error
	origStdout := os.Stdout
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	_, errW, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = errW

	err := writeHookOutput("", fmt.Errorf("test error"))

	w.Close()
	errW.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr

	if err != nil {
		t.Fatalf("writeHookOutput: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var decoded HookOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &decoded); err != nil {
		t.Fatalf("Output should be valid JSON even with error: %v (raw: %q)", err, output)
	}
}

func TestHookInputBestEffortParse(t *testing.T) {
	// Even with invalid JSON fields, the struct should parse what it can
	data := `{"hook_event_name":"Stop","unexpected_field":true}`
	var input HookInput
	err := json.Unmarshal([]byte(data), &input)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if input.HookEventName != "Stop" {
		t.Errorf("HookEventName = %q, want Stop", input.HookEventName)
	}
}

func TestHookInputEmptyJSON(t *testing.T) {
	data := `{}`
	var input HookInput
	err := json.Unmarshal([]byte(data), &input)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if input.HookEventName != "" {
		t.Errorf("HookEventName should be empty, got %q", input.HookEventName)
	}
}
