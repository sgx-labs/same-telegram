package taskqueue

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestGenerateID(t *testing.T) {
	now := time.Date(2026, 3, 5, 14, 30, 22, 0, time.UTC)

	// Reset counters for deterministic test
	lastIDSecond.Store(0)
	idCounter.Store(0)

	id := GenerateID(now)
	if !strings.HasPrefix(id, "task-20260305-143022-") {
		t.Errorf("unexpected ID format: %s", id)
	}

	// Second call in same second should increment counter
	id2 := GenerateID(now)
	if id == id2 {
		t.Error("expected different IDs for same-second calls")
	}
	if !strings.HasSuffix(id, "-001") {
		t.Errorf("expected first ID to end with -001, got %s", id)
	}
	if !strings.HasSuffix(id2, "-002") {
		t.Errorf("expected second ID to end with -002, got %s", id2)
	}

	// Different second should reset counter
	later := now.Add(time.Second)
	id3 := GenerateID(later)
	if !strings.HasSuffix(id3, "-001") {
		t.Errorf("expected counter reset for new second, got %s", id3)
	}
}

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		from    string
		to      string
		wantErr bool
	}{
		{StateQueued, StateAssigned, false},
		{StateAssigned, StateInProgress, false},
		{StateInProgress, StateReview, false},
		{StateInProgress, StateFailed, false},
		{StateReview, StateApproved, false},
		{StateReview, StateRejected, false},
		{StateApproved, StateDone, false},
		{StateRejected, StateQueued, false},
		{StateFailed, StateQueued, false},
		{StateFailed, StateDone, false},
		// Invalid transitions
		{StateQueued, StateInProgress, true},
		{StateQueued, StateDone, true},
		{StateAssigned, StateQueued, true},
		{StateInProgress, StateDone, true},
		{StateReview, StateQueued, true},
		{StateDone, StateQueued, true},
		{"bogus", StateQueued, true},
	}

	for _, tt := range tests {
		err := ValidateTransition(tt.from, tt.to)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateTransition(%s, %s): err=%v, wantErr=%v", tt.from, tt.to, err, tt.wantErr)
		}
	}
}

func TestTaskJSONRoundTrip(t *testing.T) {
	task := NewTask("Test task", "A test description", "backend", PriorityHigh, "ceo")

	data, err := task.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	parsed, err := ParseTask(data)
	if err != nil {
		t.Fatalf("ParseTask: %v", err)
	}

	if parsed.ID != task.ID {
		t.Errorf("ID mismatch: got %s, want %s", parsed.ID, task.ID)
	}
	if parsed.Title != task.Title {
		t.Errorf("Title mismatch: got %s, want %s", parsed.Title, task.Title)
	}
	if parsed.State != StateQueued {
		t.Errorf("State mismatch: got %s, want %s", parsed.State, StateQueued)
	}
	if parsed.AssignedTo != "backend" {
		t.Errorf("AssignedTo mismatch: got %s, want backend", parsed.AssignedTo)
	}
	if parsed.Priority != PriorityHigh {
		t.Errorf("Priority mismatch: got %s, want %s", parsed.Priority, PriorityHigh)
	}
	if len(parsed.History) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(parsed.History))
	}

	// Verify it's valid JSON
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
}

func TestTaskTransition(t *testing.T) {
	task := NewTask("Test", "desc", "", PriorityNormal, "ceo")

	// Valid transition
	err := task.Transition(StateAssigned, "dispatcher", "Matched to backend")
	if err != nil {
		t.Fatalf("valid transition failed: %v", err)
	}
	if task.State != StateAssigned {
		t.Errorf("state should be assigned, got %s", task.State)
	}
	if len(task.History) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(task.History))
	}

	// Invalid transition
	err = task.Transition(StateDone, "ceo", "skip ahead")
	if err == nil {
		t.Error("expected error for invalid transition, got nil")
	}
	if task.State != StateAssigned {
		t.Error("state should not have changed on invalid transition")
	}
}

func TestNewTaskDefaults(t *testing.T) {
	task := NewTask("Title", "Desc", "", "", "ceo")

	if task.Priority != PriorityNormal {
		t.Errorf("expected default priority normal, got %s", task.Priority)
	}
	if task.MaxAttempts != 2 {
		t.Errorf("expected max_attempts 2, got %d", task.MaxAttempts)
	}
	if task.TokenBudget != 100000 {
		t.Errorf("expected token_budget 100000, got %d", task.TokenBudget)
	}
	if task.Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", task.Attempt)
	}
	if task.Repo != "same-telegram" {
		t.Errorf("expected repo same-telegram, got %s", task.Repo)
	}
}
