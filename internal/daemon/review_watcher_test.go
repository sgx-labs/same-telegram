package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractSummary_StructuredHeaders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-review.md")

	content := `Agent: Backend Dev
Summary: Implemented new API endpoint for user management
Priority: High
Status: Pending Review

## Details
This PR adds the /api/users endpoint with CRUD operations.
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	summary := extractSummary(path)

	// Should extract the structured header lines
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
	for _, want := range []string{"Agent:", "Summary:", "Priority:", "Status:"} {
		if !contains(summary, want) {
			t.Errorf("summary missing %q: got %q", want, summary)
		}
	}
}

func TestExtractSummary_PlainText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.txt")

	content := "Daily report for 2026-03-05\nAll systems operational.\nNo incidents.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	summary := extractSummary(path)

	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
	if !contains(summary, "Daily report") {
		t.Errorf("expected summary to contain 'Daily report', got %q", summary)
	}
}

func TestExtractSummary_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.md")

	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	summary := extractSummary(path)
	if summary != "(empty file)" {
		t.Errorf("expected '(empty file)', got %q", summary)
	}
}

func TestExtractSummary_LongFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "long.md")

	// Create content longer than 500 chars
	content := "Title: Test Report\n"
	for i := 0; i < 100; i++ {
		content += "This is a line of padding text that makes the file very long.\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	summary := extractSummary(path)
	if !contains(summary, "Title:") {
		t.Errorf("expected summary to contain 'Title:', got %q", summary)
	}
}

func TestExtractSummary_MissingFile(t *testing.T) {
	summary := extractSummary("/nonexistent/file.md")
	if summary != "(could not read file)" {
		t.Errorf("expected error message, got %q", summary)
	}
}

func TestReviewCategory_String(t *testing.T) {
	tests := []struct {
		cat  ReviewCategory
		want string
	}{
		{CategoryReview, "Review"},
		{CategoryDecision, "Decision"},
		{CategoryReport, "Report"},
		{CategoryReply, "Reply"},
		{ReviewCategory("custom"), "custom"},
	}

	for _, tt := range tests {
		if got := tt.cat.String(); got != tt.want {
			t.Errorf("ReviewCategory(%q).String() = %q, want %q", tt.cat, got, tt.want)
		}
	}
}

func TestSeedExisting(t *testing.T) {
	dir := t.TempDir()
	reviewDir := filepath.Join(dir, "reviews", "pending")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a pre-existing file
	existing := filepath.Join(reviewDir, "old-review.md")
	if err := os.WriteFile(existing, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	rw := &ReviewWatcher{
		dirs: []watchedDir{
			{Path: reviewDir, Category: CategoryReview},
		},
		seen: make(map[string]bool),
	}

	rw.seedExisting()

	if !rw.seen[existing] {
		t.Error("expected existing file to be marked as seen")
	}
}

func TestCategoryFor(t *testing.T) {
	dir := t.TempDir()
	reviewDir := filepath.Join(dir, "reviews", "pending")
	decisionDir := filepath.Join(dir, "decisions")
	replyDir := filepath.Join(dir, "messages", "inbound")

	for _, d := range []string{reviewDir, decisionDir, replyDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	rw := &ReviewWatcher{
		dirs: []watchedDir{
			{Path: reviewDir, Category: CategoryReview},
			{Path: decisionDir, Category: CategoryDecision},
			{Path: replyDir, Category: CategoryReply},
		},
		seen: make(map[string]bool),
	}

	tests := []struct {
		path string
		want ReviewCategory
	}{
		{filepath.Join(reviewDir, "test.md"), CategoryReview},
		{filepath.Join(decisionDir, "test.md"), CategoryDecision},
		{filepath.Join(replyDir, "reply.json"), CategoryReply},
		{filepath.Join(dir, "unknown", "test.md"), ""},
	}

	for _, tt := range tests {
		got := rw.categoryFor(tt.path)
		if got != tt.want {
			t.Errorf("categoryFor(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestFormatReviewNotification(t *testing.T) {
	text := FormatReviewNotification("Decision", "approve-budget.md", "Summary: Budget approval needed", true, false)

	if !contains(text, "Decision") {
		t.Error("expected notification to contain category")
	}
	if !contains(text, "approve-budget.md") {
		t.Error("expected notification to contain filename")
	}
	if !contains(text, "Budget approval") {
		t.Error("expected notification to contain summary")
	}
	if !contains(text, "/decisions") {
		t.Error("expected notification to contain /decisions command hint")
	}
}

func TestFormatReviewNotification_Review(t *testing.T) {
	text := FormatReviewNotification("Review", "api-changes.md", "Summary: New API endpoint", false, false)

	if !contains(text, "/review api-changes") {
		t.Errorf("expected review command hint, got: %s", text)
	}
}

func TestFormatReviewNotification_Reply(t *testing.T) {
	text := FormatReviewNotification("Reply", "reply-20260305.json", "Body: acknowledged", false, true)

	if !contains(text, "/messages") {
		t.Errorf("expected /messages command hint, got: %s", text)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
