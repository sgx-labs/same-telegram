package taskqueue

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTestDirs creates a temp directory and sets SAME_COMPANY_HQ to point to it.
// Returns a cleanup function.
func setupTestDirs(t *testing.T) func() {
	t.Helper()
	tmpDir := t.TempDir()
	old := os.Getenv("SAME_COMPANY_HQ")
	os.Setenv("SAME_COMPANY_HQ", tmpDir)
	return func() {
		if old == "" {
			os.Unsetenv("SAME_COMPANY_HQ")
		} else {
			os.Setenv("SAME_COMPANY_HQ", old)
		}
	}
}

func TestEnsureDirs(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	// Verify all state directories exist
	for _, state := range AllStates {
		dir := StatePath(state)
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("state dir %s not created: %v", state, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("state dir %s is not a directory", state)
		}
	}

	// Verify logs directory
	logsDir := LogsDir()
	if _, err := os.Stat(logsDir); err != nil {
		t.Errorf("logs dir not created: %v", err)
	}
}

func TestCreateAndRead(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	task := NewTask("Test task", "Do something", "backend", PriorityNormal, "ceo")

	if err := Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify file exists in queue/
	path := filepath.Join(StatePath(StateQueued), task.Filename())
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("task file not found: %v", err)
	}

	// Read it back
	readTask, state, err := Read(task.ID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state != StateQueued {
		t.Errorf("expected state queued, got %s", state)
	}
	if readTask.Title != "Test task" {
		t.Errorf("title mismatch: got %s", readTask.Title)
	}
	if readTask.AssignedTo != "backend" {
		t.Errorf("assigned_to mismatch: got %s", readTask.AssignedTo)
	}
}

func TestList(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	// Create 3 tasks
	for i := 0; i < 3; i++ {
		task := NewTask("Task", "desc", "", PriorityNormal, "ceo")
		if err := Create(task); err != nil {
			t.Fatalf("Create task %d: %v", i, err)
		}
	}

	tasks, err := List(StateQueued)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}

	// Empty state directory
	tasks, err = List(StateAssigned)
	if err != nil {
		t.Fatalf("List assigned: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks in assigned, got %d", len(tasks))
	}
}

func TestListAll(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	task := NewTask("Task 1", "desc", "", PriorityNormal, "ceo")
	if err := Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	all, err := ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 task, got %d", len(all))
	}
}

func TestMove(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	task := NewTask("Test move", "desc", "", PriorityNormal, "ceo")
	if err := Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Move from queued to assigned
	err := Move(task.ID, StateQueued, StateAssigned, "dispatcher", "Matched to backend")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}

	// Verify it's no longer in queue/
	queuePath := filepath.Join(StatePath(StateQueued), task.Filename())
	if _, err := os.Stat(queuePath); !os.IsNotExist(err) {
		t.Error("task file should not exist in queue/ after move")
	}

	// Verify it's in assigned/
	readTask, state, err := Read(task.ID)
	if err != nil {
		t.Fatalf("Read after move: %v", err)
	}
	if state != StateAssigned {
		t.Errorf("expected state assigned, got %s", state)
	}
	if readTask.State != StateAssigned {
		t.Errorf("task state field should be assigned, got %s", readTask.State)
	}
	// Should have 2 history entries (creation + move)
	if len(readTask.History) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(readTask.History))
	}
}

func TestMoveInvalidTransition(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	task := NewTask("Test invalid", "desc", "", PriorityNormal, "ceo")
	if err := Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Try invalid transition: queued -> done
	err := Move(task.ID, StateQueued, StateDone, "ceo", "skip")
	if err == nil {
		t.Error("expected error for invalid transition")
	}

	// Task should still be in queue
	_, state, err := Read(task.ID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state != StateQueued {
		t.Errorf("task should still be queued, got %s", state)
	}
}

func TestReadNotFound(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	_, _, err := Read("task-nonexistent-000")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}
