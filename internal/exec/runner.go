package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// RunSame executes a `same` CLI command and returns its stdout.
func RunSame(args ...string) (string, error) {
	return RunSameCtx(context.Background(), args...)
}

// RunSameCtx executes a `same` CLI command with context.
func RunSameCtx(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "same", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("same %v: %w\nstderr: %s", args, err, stderr.String())
	}
	return stdout.String(), nil
}

// RunSameJSON executes a `same` CLI command and unmarshals JSON output.
func RunSameJSON(result any, args ...string) error {
	out, err := RunSame(args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(out), result); err != nil {
		return fmt.Errorf("parse same output: %w\nraw: %s", err, out)
	}
	return nil
}

// Status represents the output of `same status --json`.
type Status struct {
	Vault     string `json:"vault"`
	Path      string `json:"path"`
	Notes     int    `json:"notes"`
	Decisions int    `json:"decisions"`
	Sessions  int    `json:"sessions"`
	IndexAge  string `json:"index_age"`
	Healthy   bool   `json:"healthy"`
}

// GetStatus runs `same status --json`.
func GetStatus() (*Status, error) {
	var s Status
	if err := RunSameJSON(&s, "status", "--json"); err != nil {
		// Fallback to text output
		out, err2 := RunSame("status")
		if err2 != nil {
			return nil, err
		}
		return &Status{Vault: out}, nil
	}
	return &s, nil
}

// SearchResult represents a single result from `same search --json`.
type SearchResult struct {
	Title   string  `json:"title"`
	Path    string  `json:"path"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
	Type    string  `json:"type"`
}

// Search runs `same search --json <query>` and returns raw output.
func Search(query string) (string, error) {
	return RunSame("search", "--json", query)
}

// SearchJSON runs `same search --json <query>` and returns parsed results.
func SearchJSON(query string) ([]SearchResult, error) {
	var results []SearchResult
	if err := RunSameJSON(&results, "search", "--json", query); err != nil {
		return nil, err
	}
	return results, nil
}

// Ask runs `same ask <question>`.
func Ask(question string) (string, error) {
	return RunSame("ask", question)
}

// Doctor runs `same doctor`.
func Doctor() (string, error) {
	return RunSame("doctor")
}

// ClaudeOptions configures a Claude CLI invocation.
type ClaudeOptions struct {
	// MCPConfigPath is the path to an MCP config JSON file.
	// If non-empty and the file exists, --mcp-config is passed.
	MCPConfigPath string

	// SessionID is a previous session ID to resume.
	// If non-empty, --resume <id> is passed.
	SessionID string
}

// ClaudeResult holds the response from a Claude CLI invocation.
type ClaudeResult struct {
	// Text is the response text from Claude.
	Text string

	// SessionID is the session ID returned by Claude (for resuming later).
	SessionID string
}

// claudeJSONOutput represents the JSON output from claude --output-format json.
type claudeJSONOutput struct {
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
	IsError   bool   `json:"is_error"`
}

// DefaultMCPConfigPath is the default location for the SAME MCP server config.
const DefaultMCPConfigPath = "/workspace/code/statelessagent/.mcp.json"

// RunClaude sends a prompt to the `claude` CLI and returns the response.
// It accepts optional ClaudeOptions for MCP config and session resumption.
func RunClaude(prompt string, opts ...ClaudeOptions) (string, error) {
	result, err := RunClaudeWithSession(prompt, opts...)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// RunClaudeWithSession sends a prompt to the `claude` CLI and returns
// both the response text and the session ID for future resumption.
func RunClaudeWithSession(prompt string, opts ...ClaudeOptions) (*ClaudeResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var opt ClaudeOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	// Build args: --print --dangerously-skip-permissions --output-format json
	args := []string{"--print", "--dangerously-skip-permissions", "--output-format", "json"}

	// Add MCP config if specified and file exists
	mcpPath := opt.MCPConfigPath
	if mcpPath == "" {
		mcpPath = DefaultMCPConfigPath
	}
	if _, err := os.Stat(mcpPath); err == nil {
		args = append(args, "--mcp-config", mcpPath)
	}

	// Add session resumption if we have a session ID
	if opt.SessionID != "" {
		args = append(args, "--resume", opt.SessionID)
	}

	// Prompt goes last
	args = append(args, "-p", prompt)

	cmd := exec.CommandContext(ctx, "claude", args...)

	// Filter CLAUDECODE env var to prevent nested session error
	filtered := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if name, _, _ := strings.Cut(e, "="); name != "CLAUDECODE" {
			filtered = append(filtered, e)
		}
	}
	cmd.Env = filtered

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("claude: %w\nstderr: %s", err, stderr.String())
	}

	raw := stdout.String()

	// Try to parse JSON output to extract session_id
	var jsonOut claudeJSONOutput
	if err := json.Unmarshal([]byte(raw), &jsonOut); err == nil {
		if jsonOut.IsError {
			return nil, fmt.Errorf("claude error: %s", jsonOut.Result)
		}
		return &ClaudeResult{
			Text:      jsonOut.Result,
			SessionID: jsonOut.SessionID,
		}, nil
	}

	// Fallback: return raw text if JSON parsing fails
	return &ClaudeResult{
		Text: raw,
	}, nil
}
