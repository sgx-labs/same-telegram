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
	Vault      string `json:"vault"`
	Path       string `json:"path"`
	Notes      int    `json:"notes"`
	Decisions  int    `json:"decisions"`
	Sessions   int    `json:"sessions"`
	IndexAge   string `json:"index_age"`
	Healthy    bool   `json:"healthy"`
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

// RunClaude sends a prompt to the `claude` CLI and returns the response.
func RunClaude(prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "--print", "--dangerously-skip-permissions", "-p", prompt)
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
		return "", fmt.Errorf("claude: %w\nstderr: %s", err, stderr.String())
	}
	return stdout.String(), nil
}
