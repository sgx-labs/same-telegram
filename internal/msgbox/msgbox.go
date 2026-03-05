package msgbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Directories under company-hq/messages/
const (
	outboundDir   = "messages/outbound"
	processedDir  = "messages/outbound/processed"
	inboundDir    = "messages/inbound"
)

// Message types for agent-to-CEO communication.
const (
	MsgTypeQuestion = "question"
	MsgTypeStatus   = "status"
	MsgTypeBlocker  = "blocker"
)

// Message is the JSON format agents write to the outbound directory.
type Message struct {
	From      string    `json:"from"`
	Type      string    `json:"type"`             // "question", "status", "blocker"
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Timestamp time.Time `json:"timestamp"`
}

// Reply is written by the daemon to the inbound directory when the CEO responds.
type Reply struct {
	To        string    `json:"to"`
	InReplyTo string    `json:"in_reply_to"` // original message filename
	Body      string    `json:"body"`
	Timestamp time.Time `json:"timestamp"`
}

// CompanyHQDir returns the path to company-hq.
// Uses SAME_COMPANY_HQ env var if set, otherwise defaults to ~/Projects/same-company/company-hq.
func CompanyHQDir() string {
	if dir := os.Getenv("SAME_COMPANY_HQ"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Projects", "same-company", "company-hq")
}

// OutboundDir returns the full path to the outbound message directory.
func OutboundDir() string {
	return filepath.Join(CompanyHQDir(), outboundDir)
}

// InboundDir returns the full path to the inbound reply directory.
func InboundDir() string {
	return filepath.Join(CompanyHQDir(), inboundDir)
}

// ProcessedDir returns the full path to the processed message directory.
func ProcessedDir() string {
	return filepath.Join(CompanyHQDir(), processedDir)
}

// EnsureDirs creates the outbound, processed, and inbound directories if needed.
func EnsureDirs() error {
	for _, dir := range []string{OutboundDir(), ProcessedDir(), InboundDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}

// ReadMessage reads and parses a message JSON file.
func ReadMessage(path string) (*Message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse message %s: %w", filepath.Base(path), err)
	}
	return &m, nil
}

// WriteReply writes a CEO reply to the inbound directory.
func WriteReply(reply *Reply) error {
	if err := EnsureDirs(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(reply, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal reply: %w", err)
	}

	filename := fmt.Sprintf("reply-%s.json", time.Now().Format("20060102-150405"))
	path := filepath.Join(InboundDir(), filename)

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write reply: %w", err)
	}
	return nil
}

// ArchiveMessage moves a processed message file to the processed/ subdirectory.
func ArchiveMessage(path string) error {
	if err := os.MkdirAll(ProcessedDir(), 0o755); err != nil {
		return fmt.Errorf("create processed dir: %w", err)
	}
	dst := filepath.Join(ProcessedDir(), filepath.Base(path))
	return os.Rename(path, dst)
}

// WriteMessage writes an agent message to the outbound directory.
func WriteMessage(msg *Message) (string, error) {
	if err := EnsureDirs(); err != nil {
		return "", err
	}

	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	data, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal message: %w", err)
	}

	filename := fmt.Sprintf("%s-%s.json", msg.From, time.Now().Format("20060102-150405"))
	path := filepath.Join(OutboundDir(), filename)

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write message: %w", err)
	}
	return filename, nil
}
