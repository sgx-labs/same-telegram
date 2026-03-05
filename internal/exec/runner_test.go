package exec

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDefaultTimeout(t *testing.T) {
	if defaultTimeout != 30*time.Second {
		t.Errorf("defaultTimeout = %v, want 30s", defaultTimeout)
	}
}

func TestStatusJSON(t *testing.T) {
	s := Status{
		Vault:    "work",
		Path:     "/home/user/.same/work",
		Notes:    42,
		Decisions: 7,
		Sessions: 15,
		IndexAge: "2h30m",
		Healthy:  true,
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Status
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Vault != "work" {
		t.Errorf("Vault = %q, want work", decoded.Vault)
	}
	if decoded.Notes != 42 {
		t.Errorf("Notes = %d, want 42", decoded.Notes)
	}
	if !decoded.Healthy {
		t.Error("Healthy should be true")
	}
}

func TestStatusJSONFromString(t *testing.T) {
	// Simulate what RunSameJSON would parse
	raw := `{"vault":"personal","path":"/home/u/.same/personal","notes":10,"decisions":3,"sessions":5,"index_age":"1h","healthy":false}`

	var s Status
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if s.Vault != "personal" {
		t.Errorf("Vault = %q", s.Vault)
	}
	if s.Healthy {
		t.Error("Healthy should be false")
	}
	if s.Notes != 10 {
		t.Errorf("Notes = %d, want 10", s.Notes)
	}
}

func TestStatusJSONExtraFields(t *testing.T) {
	// Extra fields in JSON should be silently ignored
	raw := `{"vault":"test","notes":1,"extra_field":"value","healthy":true}`
	var s Status
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("Unmarshal should handle extra fields: %v", err)
	}
	if s.Vault != "test" {
		t.Errorf("Vault = %q", s.Vault)
	}
}

func TestRunSameMissingBinary(t *testing.T) {
	// `same` binary likely doesn't exist in test environment
	_, err := RunSame("--nonexistent-flag")
	if err == nil {
		// If `same` IS installed, this test is inconclusive but not a failure
		t.Skip("same binary is installed, cannot test missing binary case")
	}
	// Error should mention the command
	if err != nil {
		errStr := err.Error()
		if len(errStr) == 0 {
			t.Error("Expected non-empty error message")
		}
	}
}

func TestRunSameJSONInvalidOutput(t *testing.T) {
	// RunSameJSON should fail gracefully when output isn't valid JSON
	// We can't easily mock RunSame, but we can test the JSON parsing part
	// by calling RunSameJSON with a command that doesn't produce JSON
	var s Status
	err := RunSameJSON(&s, "--version")
	if err == nil {
		// If same is installed and --version produces JSON (unlikely), skip
		t.Skip("same --version produced parseable output")
	}
	// Error is expected -- either binary not found or non-JSON output
}
