package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/sgx-labs/same-telegram/internal/notify"
)

// HookInput matches the JSON from Claude Code hooks (same as SAME's HookInput).
type HookInput struct {
	Prompt         string `json:"prompt,omitempty"`
	TranscriptPath string `json:"transcript_path,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	HookEventName  string `json:"hook_event_name,omitempty"`
}

// HookOutput matches the JSON expected by Claude Code hooks.
type HookOutput struct {
	SystemMessage string `json:"systemMessage,omitempty"`
}

var hookCmd = &cobra.Command{
	Use:   "hook [event-name]",
	Short: "Plugin mode: forward hook events to the daemon via unix socket",
	Long: `Receives Claude Code hook JSON on stdin and forwards it to the
running same-telegram daemon via unix socket. This is the fast path
(stdin -> socket, sub-millisecond) used as a Claude Code hook plugin.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runHook,
}

func runHook(cmd *cobra.Command, args []string) error {
	// Read stdin (Claude Code hook input)
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 10*1024*1024))
	if err != nil {
		return writeHookOutput("", err)
	}

	var input HookInput
	if len(data) > 0 {
		json.Unmarshal(data, &input) // best-effort parse
	}

	// Determine event type
	eventName := input.HookEventName
	if len(args) > 0 {
		eventName = args[0]
	}

	// Build notification
	n := &notify.Notification{
		Timestamp: time.Now(),
		SessionID: input.SessionID,
	}

	switch eventName {
	case "Stop":
		n.Type = notify.TypeSessionEnd
		n.Summary = "Session ended"
		if input.TranscriptPath != "" {
			n.Details = fmt.Sprintf("Transcript: %s", input.TranscriptPath)
		}
	case "UserPromptSubmit":
		// Don't notify on every prompt — too noisy
		return writeHookOutput("", nil)
	case "SessionStart":
		// Could optionally notify, but keep quiet for now
		return writeHookOutput("", nil)
	default:
		n.Type = notify.TypeCustom
		n.Summary = fmt.Sprintf("Hook event: %s", eventName)
	}

	// Send to daemon (non-blocking if daemon isn't running)
	if err := notify.Send(n); err != nil {
		// Log but don't fail the hook — daemon may just be offline
		fmt.Fprintf(os.Stderr, "same-telegram: warning: %v\n", err)
	}

	return writeHookOutput("", nil)
}

func writeHookOutput(msg string, err error) error {
	if err != nil {
		fmt.Fprintf(os.Stderr, "same-telegram: %v\n", err)
	}

	output := HookOutput{}
	if msg != "" {
		output.SystemMessage = msg
	}

	return json.NewEncoder(os.Stdout).Encode(output)
}
