package bot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdTeam(t *testing.T) {
	text, err := cmdTeam()
	if err != nil {
		t.Fatalf("cmdTeam() error: %v", err)
	}
	if !strings.Contains(text, "Agent Team Status") {
		t.Errorf("Expected 'Agent Team Status' in output, got: %s", text)
	}
	if !strings.Contains(text, "Pending:") {
		t.Errorf("Expected 'Pending:' in output, got: %s", text)
	}
}

func TestCmdDecisionsEmpty(t *testing.T) {
	text, decisions, err := cmdDecisions()
	if err != nil {
		t.Fatalf("cmdDecisions() error: %v", err)
	}
	// The pending dir may or may not have files, but it should not error
	if text == "" {
		t.Error("Expected non-empty text from cmdDecisions")
	}
	_ = decisions
}

func TestCmdDecisionsWithFiles(t *testing.T) {
	// Create a temp directory to simulate pending decisions
	tmpDir := t.TempDir()
	pendingDir := filepath.Join(tmpDir, "decisions", "pending")
	if err := os.MkdirAll(pendingDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := "# Test Decision\n\nShould we adopt Go 1.25?\n\n## Rationale\nPerformance improvements.\n"
	if err := os.WriteFile(filepath.Join(pendingDir, "test-decision.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Read directly using the helper functions
	title := decisionTitle(content, "test-decision.md")
	if title != "Test Decision" {
		t.Errorf("decisionTitle = %q, want %q", title, "Test Decision")
	}

	preview := decisionPreview(content)
	if !strings.Contains(preview, "Should we adopt") {
		t.Errorf("decisionPreview = %q, want it to contain 'Should we adopt'", preview)
	}
}

func TestDecisionTitleFromHeading(t *testing.T) {
	tests := []struct {
		content  string
		filename string
		want     string
	}{
		{"# My Decision\nSome body", "file.md", "My Decision"},
		{"# Decision: Use Redis\nDetails here", "use-redis.md", "Decision: Use Redis"},
		{"No heading\nJust text", "fallback.md", "fallback"},
		{"", "empty.md", "empty"},
	}
	for _, tt := range tests {
		got := decisionTitle(tt.content, tt.filename)
		if got != tt.want {
			t.Errorf("decisionTitle(%q, %q) = %q, want %q", tt.content[:min(len(tt.content), 20)], tt.filename, got, tt.want)
		}
	}
}

func TestDecisionPreview(t *testing.T) {
	tests := []struct {
		content string
		want    string
	}{
		{"# Title\nFirst line of body", "First line of body"},
		{"# Title\n\nSecond paragraph", "Second paragraph"},
		{"# Title\n---\nAfter rule", "After rule"},
		{"# Title only\n", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := decisionPreview(tt.content)
		if got != tt.want {
			t.Errorf("decisionPreview(%q) = %q, want %q", tt.content[:min(len(tt.content), 20)], got, tt.want)
		}
	}
}

func TestDecisionPreviewTruncation(t *testing.T) {
	long := strings.Repeat("x", 200)
	content := "# Title\n" + long
	got := decisionPreview(content)
	if len(got) > 124 { // 120 + "..."
		t.Errorf("Preview should be truncated, got length %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("Truncated preview should end with '...', got: %q", got)
	}
}

func TestCmdAnnounceEmpty(t *testing.T) {
	text, err := cmdAnnounce("")
	if err != nil {
		t.Fatalf("cmdAnnounce('') error: %v", err)
	}
	if !strings.Contains(text, "Usage:") {
		t.Errorf("Expected usage hint, got: %s", text)
	}
}

func TestCmdAnnounceWhitespace(t *testing.T) {
	text, err := cmdAnnounce("   ")
	if err != nil {
		t.Fatalf("cmdAnnounce error: %v", err)
	}
	if !strings.Contains(text, "Usage:") {
		t.Errorf("Expected usage hint for whitespace-only input, got: %s", text)
	}
}

func TestCmdAnnounceWritesFile(t *testing.T) {
	// This test writes to the real announcements dir.
	// Skip if company-hq doesn't exist.
	dir := filepath.Join(companyHQDir(), "announcements")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skip("company-hq/announcements/ not found, skipping write test")
	}

	text, err := cmdAnnounce("Test announcement from unit test")
	if err != nil {
		t.Fatalf("cmdAnnounce error: %v", err)
	}
	if !strings.Contains(text, "Announcement Posted") {
		t.Errorf("Expected 'Announcement Posted' in output, got: %s", text)
	}

	// Clean up: find and remove the test file
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), "Test announcement from unit test") {
			os.Remove(path)
		}
	}
}

func TestDecisionKeyboard(t *testing.T) {
	kb := DecisionKeyboard("pricing-model.md")
	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(kb.InlineKeyboard))
	}
	row := kb.InlineKeyboard[0]
	if len(row) != 2 {
		t.Fatalf("Expected 2 buttons, got %d", len(row))
	}

	approveData := callbackData(row[0].CallbackData)
	if approveData != "approve:pricing-model" {
		t.Errorf("Approve button data = %q, want %q", approveData, "approve:pricing-model")
	}

	rejectData := callbackData(row[1].CallbackData)
	if rejectData != "reject:pricing-model" {
		t.Errorf("Reject button data = %q, want %q", rejectData, "reject:pricing-model")
	}
}

func TestDecisionKeyboardStripsMdExtension(t *testing.T) {
	kb := DecisionKeyboard("my-decision.md")
	row := kb.InlineKeyboard[0]
	data := callbackData(row[0].CallbackData)
	if strings.Contains(data, ".md") {
		t.Errorf("Callback data should not contain .md extension: %q", data)
	}
}

func TestCountFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file1.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, ".gitkeep"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0o644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0o755)

	count, err := countFiles(dir)
	if err != nil {
		t.Fatalf("countFiles error: %v", err)
	}
	if count != 2 {
		t.Errorf("countFiles = %d, want 2 (file1.md + file2.txt)", count)
	}
}

func TestCountFilesNonexistent(t *testing.T) {
	_, err := countFiles("/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent directory")
	}
}

func TestCountFilesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	count, err := countFiles(dir)
	if err != nil {
		t.Fatalf("countFiles error: %v", err)
	}
	if count != 0 {
		t.Errorf("countFiles empty dir = %d, want 0", count)
	}
}

func TestDecisionKeyboardNoExtension(t *testing.T) {
	kb := DecisionKeyboard("no-extension")
	row := kb.InlineKeyboard[0]
	approveData := callbackData(row[0].CallbackData)
	if approveData != "approve:no-extension" {
		t.Errorf("Approve data = %q, want %q", approveData, "approve:no-extension")
	}
}

func TestDecisionTitleMultipleHeadings(t *testing.T) {
	content := "# First Heading\n## Second Heading\nBody text"
	got := decisionTitle(content, "file.md")
	if got != "First Heading" {
		t.Errorf("Should use first heading, got %q", got)
	}
}

func TestDecisionTitleLeadingBlankLines(t *testing.T) {
	content := "\n\n# Heading After Blanks\nBody"
	got := decisionTitle(content, "file.md")
	if got != "Heading After Blanks" {
		t.Errorf("Should find heading after blank lines, got %q", got)
	}
}

func TestDecisionPreviewSkipsSubheadings(t *testing.T) {
	content := "# Title\n## Subtitle\nActual body text"
	got := decisionPreview(content)
	if got != "Actual body text" {
		t.Errorf("Should skip subheadings, got %q", got)
	}
}

func TestDecisionPreviewSkipsHorizontalRules(t *testing.T) {
	content := "# Title\n---\n---\nBody after rules"
	got := decisionPreview(content)
	if got != "Body after rules" {
		t.Errorf("Should skip horizontal rules, got %q", got)
	}
}

func TestCmdTeamOutputFormat(t *testing.T) {
	text, err := cmdTeam()
	if err != nil {
		t.Fatalf("cmdTeam error: %v", err)
	}
	// Verify all expected sections are present
	for _, expected := range []string{"Pending:", "Approved:", "Rejected:", "Announcements:"} {
		if !strings.Contains(text, expected) {
			t.Errorf("Missing %q in team output: %s", expected, text)
		}
	}
}

func TestCmdAnnounceWritesToTempDir(t *testing.T) {
	// Use HOME override to write to temp dir instead of real company-hq
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create the announcements directory
	annDir := filepath.Join(tmpDir, "Projects", "same-company", "company-hq", "announcements")
	os.MkdirAll(annDir, 0o755)

	text, err := cmdAnnounce("Important update for all agents")
	if err != nil {
		t.Fatalf("cmdAnnounce error: %v", err)
	}
	if !strings.Contains(text, "Announcement Posted") {
		t.Errorf("Expected 'Announcement Posted', got: %s", text)
	}

	// Verify file was written
	entries, err := os.ReadDir(annDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	found := false
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(annDir, e.Name()))
		if strings.Contains(string(data), "Important update for all agents") {
			found = true
			// Verify file format
			if !strings.Contains(string(data), "# CEO Announcement") {
				t.Error("Announcement file missing header")
			}
			if !strings.Contains(string(data), "Posted:") {
				t.Error("Announcement file missing timestamp")
			}
		}
	}
	if !found {
		t.Error("Announcement file not found in temp directory")
	}
}

func TestCmdAnnounceMarkdownSpecialChars(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	annDir := filepath.Join(tmpDir, "Projects", "same-company", "company-hq", "announcements")
	os.MkdirAll(annDir, 0o755)

	text, err := cmdAnnounce("Use *bold* and _italic_ in code")
	if err != nil {
		t.Fatalf("cmdAnnounce error: %v", err)
	}
	if !strings.Contains(text, "Announcement Posted") {
		t.Errorf("Expected success, got: %s", text)
	}
}

func TestCmdAnnounceFilenameFormat(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	annDir := filepath.Join(tmpDir, "Projects", "same-company", "company-hq", "announcements")
	os.MkdirAll(annDir, 0o755)

	_, err := cmdAnnounce("Test")
	if err != nil {
		t.Fatalf("cmdAnnounce error: %v", err)
	}

	entries, _ := os.ReadDir(annDir)
	if len(entries) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(entries))
	}
	name := entries[0].Name()
	if !strings.HasSuffix(name, ".md") {
		t.Errorf("Announcement filename should end with .md, got: %s", name)
	}
	// Filename should be timestamp-based: YYYY-MM-DD-HHMMSS.md
	if len(name) < 20 {
		t.Errorf("Filename too short for timestamp format: %s", name)
	}
}

func TestHelpTextIncludesTeamCommands(t *testing.T) {
	text := helpText()
	for _, cmd := range []string{"/team", "/decisions", "/announce"} {
		if !strings.Contains(text, cmd) {
			t.Errorf("helpText missing %s", cmd)
		}
	}
}

func TestCmdDecisionsSkipsGitkeep(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	pendingDir := filepath.Join(tmpDir, "Projects", "same-company", "company-hq", "decisions", "pending")
	os.MkdirAll(pendingDir, 0o755)

	// Only a .gitkeep file
	os.WriteFile(filepath.Join(pendingDir, ".gitkeep"), []byte(""), 0o644)

	text, decisions, err := cmdDecisions()
	if err != nil {
		t.Fatalf("cmdDecisions error: %v", err)
	}
	if len(decisions) != 0 {
		t.Errorf("Expected 0 decisions (only .gitkeep), got %d", len(decisions))
	}
	if !strings.Contains(text, "No pending decisions") {
		t.Errorf("Expected 'No pending decisions' message, got: %s", text)
	}
}

func TestCmdDecisionsSkipsDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	pendingDir := filepath.Join(tmpDir, "Projects", "same-company", "company-hq", "decisions", "pending")
	os.MkdirAll(pendingDir, 0o755)

	// A subdirectory should be skipped
	os.Mkdir(filepath.Join(pendingDir, "subdir"), 0o755)
	// And a real decision file
	os.WriteFile(filepath.Join(pendingDir, "real.md"), []byte("# Real Decision\nDetails"), 0o644)

	_, decisions, err := cmdDecisions()
	if err != nil {
		t.Fatalf("cmdDecisions error: %v", err)
	}
	if len(decisions) != 1 {
		t.Errorf("Expected 1 decision (subdir skipped), got %d", len(decisions))
	}
	if len(decisions) > 0 && decisions[0].Filename != "real.md" {
		t.Errorf("Expected real.md, got %s", decisions[0].Filename)
	}
}

func TestCmdDecisionsMultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	pendingDir := filepath.Join(tmpDir, "Projects", "same-company", "company-hq", "decisions", "pending")
	os.MkdirAll(pendingDir, 0o755)

	os.WriteFile(filepath.Join(pendingDir, "a.md"), []byte("# Decision A\nUse Redis"), 0o644)
	os.WriteFile(filepath.Join(pendingDir, "b.md"), []byte("# Decision B\nUse Postgres"), 0o644)
	os.WriteFile(filepath.Join(pendingDir, "c.md"), []byte("# Decision C\nUse Go"), 0o644)

	text, decisions, err := cmdDecisions()
	if err != nil {
		t.Fatalf("cmdDecisions error: %v", err)
	}
	if len(decisions) != 3 {
		t.Errorf("Expected 3 decisions, got %d", len(decisions))
	}
	if !strings.Contains(text, "Pending Decisions (3)") {
		t.Errorf("Expected count in header, got: %s", text)
	}
}

func TestCmdDecisionsNonexistentDir(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Don't create the directory
	text, decisions, err := cmdDecisions()
	if err != nil {
		t.Fatalf("Expected no error for missing dir, got: %v", err)
	}
	if len(decisions) != 0 {
		t.Errorf("Expected 0 decisions, got %d", len(decisions))
	}
	if !strings.Contains(text, "No pending decisions directory") {
		t.Errorf("Expected 'no directory' message, got: %s", text)
	}
}

func TestReadTeamConfig(t *testing.T) {
	// Uses the real config if it exists
	cfg, err := readTeamConfig()
	if err != nil {
		t.Skip("Team config not available, skipping")
	}
	if cfg.Name == "" {
		t.Error("Expected non-empty team name")
	}
	if len(cfg.Members) == 0 {
		t.Error("Expected at least one member")
	}
}

func TestReadTeamConfigFromTempDir(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create a mock config
	cfgDir := filepath.Join(tmpDir, ".claude", "teams", "same-company")
	os.MkdirAll(cfgDir, 0o755)

	cfg := teamConfig{
		Name: "test-team",
		Members: []teamMember{
			{Name: "lead", AgentType: "team-lead", Model: "claude-opus-4-6"},
			{Name: "dev", AgentType: "general-purpose", Model: "claude-sonnet-4-6"},
		},
	}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644)

	got, err := readTeamConfig()
	if err != nil {
		t.Fatalf("readTeamConfig: %v", err)
	}
	if got.Name != "test-team" {
		t.Errorf("Name = %q, want %q", got.Name, "test-team")
	}
	if len(got.Members) != 2 {
		t.Errorf("Members count = %d, want 2", len(got.Members))
	}
	if got.Members[0].Name != "lead" {
		t.Errorf("First member = %q, want %q", got.Members[0].Name, "lead")
	}
}

func TestReadTeamConfigMissing(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	_, err := readTeamConfig()
	if err == nil {
		t.Error("Expected error for missing config")
	}
}

func TestCmdTeamWithMockConfig(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create mock team config
	cfgDir := filepath.Join(tmpDir, ".claude", "teams", "same-company")
	os.MkdirAll(cfgDir, 0o755)
	cfg := teamConfig{
		Name: "my-team",
		Members: []teamMember{
			{Name: "alice", AgentType: "team-lead", Model: "claude-opus-4-6"},
			{Name: "bob", AgentType: "general-purpose", Model: "claude-sonnet-4-6"},
		},
	}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644)

	// Create company-hq dirs
	hq := filepath.Join(tmpDir, "Projects", "same-company", "company-hq")
	os.MkdirAll(filepath.Join(hq, "decisions", "pending"), 0o755)
	os.MkdirAll(filepath.Join(hq, "decisions", "approved"), 0o755)
	os.MkdirAll(filepath.Join(hq, "decisions", "rejected"), 0o755)
	os.MkdirAll(filepath.Join(hq, "announcements"), 0o755)

	text, err := cmdTeam()
	if err != nil {
		t.Fatalf("cmdTeam: %v", err)
	}

	// Should show team info
	if !strings.Contains(text, "my-team") {
		t.Errorf("Expected team name in output: %s", text)
	}
	if !strings.Contains(text, "alice") {
		t.Errorf("Expected member alice in output: %s", text)
	}
	if !strings.Contains(text, "bob") {
		t.Errorf("Expected member bob in output: %s", text)
	}
	if !strings.Contains(text, "team-lead") {
		t.Errorf("Expected agent type in output: %s", text)
	}
	if !strings.Contains(text, "Members:* 2") {
		t.Errorf("Expected member count in output: %s", text)
	}
	// Should still show decision counts
	if !strings.Contains(text, "Pending:") {
		t.Errorf("Expected decision counts in output: %s", text)
	}
}

func TestCmdTeamWithoutConfig(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create company-hq dirs but no team config
	hq := filepath.Join(tmpDir, "Projects", "same-company", "company-hq")
	os.MkdirAll(filepath.Join(hq, "decisions", "pending"), 0o755)
	os.MkdirAll(filepath.Join(hq, "announcements"), 0o755)

	text, err := cmdTeam()
	if err != nil {
		t.Fatalf("cmdTeam: %v", err)
	}

	if !strings.Contains(text, "not available") {
		t.Errorf("Expected 'not available' fallback, got: %s", text)
	}
	// Should still show decision counts
	if !strings.Contains(text, "Pending:") {
		t.Errorf("Expected decision counts even without config: %s", text)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
