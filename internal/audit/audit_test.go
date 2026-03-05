package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogReviewAction(t *testing.T) {
	// Set up temp company-hq dir
	tmpDir := t.TempDir()
	t.Setenv("SAME_COMPANY_HQ", tmpDir)

	LogReviewAction("approved", "proposal.md", "intent")
	LogReviewAction("approved", "proposal.md", "ok")

	path := filepath.Join(tmpDir, "reviews", "audit.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Read audit log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("Expected 2 lines, got %d: %s", len(lines), string(data))
	}

	// Check format: YYYY-MM-DD HH:MM:SS | ACTION | filename | result
	for _, line := range lines {
		parts := strings.Split(line, " | ")
		if len(parts) != 4 {
			t.Errorf("Expected 4 pipe-delimited parts, got %d in: %s", len(parts), line)
		}
	}

	if !strings.Contains(lines[0], "| approved | proposal.md | intent") {
		t.Errorf("First line unexpected: %s", lines[0])
	}
	if !strings.Contains(lines[1], "| approved | proposal.md | ok") {
		t.Errorf("Second line unexpected: %s", lines[1])
	}
}
