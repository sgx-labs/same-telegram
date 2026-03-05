package taskqueue

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

// Task states.
const (
	StateQueued     = "queued"
	StateAssigned   = "assigned"
	StateInProgress = "in_progress"
	StateReview     = "review"
	StateApproved   = "approved"
	StateDone       = "done"
	StateRejected   = "rejected"
	StateFailed     = "failed"
)

// AllStates lists every possible task state.
var AllStates = []string{
	StateQueued, StateAssigned, StateInProgress, StateReview,
	StateApproved, StateDone, StateRejected, StateFailed,
}

// ActiveStates lists states that represent active/pending work.
var ActiveStates = []string{
	StateQueued, StateAssigned, StateInProgress, StateReview,
}

// validTransitions defines the state machine.
// Each key maps to the set of states it can transition to.
var validTransitions = map[string]map[string]bool{
	StateQueued:     {StateAssigned: true, StateFailed: true},
	StateAssigned:   {StateInProgress: true, StateFailed: true},
	StateInProgress: {StateReview: true, StateFailed: true},
	StateReview:     {StateApproved: true, StateRejected: true},
	StateApproved:   {StateDone: true},
	StateRejected:   {StateQueued: true},
	StateFailed:     {StateQueued: true, StateDone: true},
}

// Priority levels.
const (
	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
)

// HistoryEntry records a single state transition.
type HistoryEntry struct {
	Timestamp time.Time `json:"timestamp"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	By        string    `json:"by"`
	Note      string    `json:"note"`
}

// Task represents a unit of work in the task queue.
type Task struct {
	ID               string         `json:"id"`
	Title            string         `json:"title"`
	Description      string         `json:"description"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	State            string         `json:"state"`
	AssignedTo       string         `json:"assigned_to"`
	Priority         string         `json:"priority"`
	Repo             string         `json:"repo"`
	Branch           string         `json:"branch"`
	WorktreePath     string         `json:"worktree_path"`
	ReviewFile       string         `json:"review_file"`
	CreatedBy        string         `json:"created_by"`
	Feedback         string         `json:"feedback"`
	Attempt          int            `json:"attempt"`
	MaxAttempts      int            `json:"max_attempts"`
	TokenBudget      int            `json:"token_budget"`
	TokensUsed       int            `json:"tokens_used"`
	TimeBudgetMinutes int           `json:"time_budget_minutes"`
	StartedAt        *time.Time     `json:"started_at,omitempty"`
	CompletedAt      *time.Time     `json:"completed_at,omitempty"`
	SessionID        string         `json:"session_id"`
	Error            string         `json:"error"`
	LogFile          string         `json:"log_file"`
	History          []HistoryEntry `json:"history"`
}

// idCounter is used for the NNN suffix within the same second.
var idCounter atomic.Int64

// lastIDSecond tracks the last second we generated an ID in, to reset the counter.
var lastIDSecond atomic.Int64

// GenerateID creates a task ID in the format task-YYYYMMDD-HHMMSS-NNN.
func GenerateID(now time.Time) string {
	sec := now.Unix()
	prev := lastIDSecond.Swap(sec)
	if prev != sec {
		idCounter.Store(0)
	}
	n := idCounter.Add(1)
	return fmt.Sprintf("task-%s-%03d", now.Format("20060102-150405"), n)
}

// ValidateTransition checks whether moving from one state to another is allowed.
func ValidateTransition(from, to string) error {
	targets, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("unknown state %q", from)
	}
	if !targets[to] {
		return fmt.Errorf("invalid transition: %s -> %s", from, to)
	}
	return nil
}

// AddHistory appends a history entry and updates the task's UpdatedAt timestamp.
func (t *Task) AddHistory(from, to, by, note string) {
	now := time.Now()
	t.History = append(t.History, HistoryEntry{
		Timestamp: now,
		From:      from,
		To:        to,
		By:        by,
		Note:      note,
	})
	t.UpdatedAt = now
}

// Transition moves the task to a new state if the transition is valid.
// It appends a history entry and updates timestamps.
func (t *Task) Transition(to, by, note string) error {
	if err := ValidateTransition(t.State, to); err != nil {
		return err
	}
	from := t.State
	t.State = to
	t.AddHistory(from, to, by, note)
	return nil
}

// MarshalJSON returns the JSON encoding of the task.
func (t *Task) MarshalJSON() ([]byte, error) {
	// Use an alias to avoid infinite recursion.
	type Alias Task
	return json.MarshalIndent((*Alias)(t), "", "  ")
}

// NewTask creates a new task with sensible defaults.
func NewTask(title, description, assignedTo, priority, createdBy string) *Task {
	now := time.Now()
	if priority == "" {
		priority = PriorityNormal
	}

	t := &Task{
		ID:                GenerateID(now),
		Title:             title,
		Description:       description,
		CreatedAt:         now,
		UpdatedAt:         now,
		State:             StateQueued,
		AssignedTo:        assignedTo,
		Priority:          priority,
		Repo:              "same-telegram",
		CreatedBy:         createdBy,
		Attempt:           1,
		MaxAttempts:       2,
		TokenBudget:       100000,
		TimeBudgetMinutes: 30,
	}

	t.AddHistory("", StateQueued, createdBy, "Created via /task")
	return t
}

// Filename returns the JSON filename for this task.
func (t *Task) Filename() string {
	return t.ID + ".json"
}

// ParseTask unmarshals a Task from JSON bytes.
func ParseTask(data []byte) (*Task, error) {
	var t Task
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse task: %w", err)
	}
	return &t, nil
}

// ElapsedTime returns how long the task has been active (since creation).
func (t *Task) ElapsedTime() time.Duration {
	if t.StartedAt != nil {
		return time.Since(*t.StartedAt)
	}
	return time.Since(t.CreatedAt)
}

// StateDirName returns the directory name for the current state.
func StateDirName(state string) string {
	return state
}
