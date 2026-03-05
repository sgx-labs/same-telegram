package taskqueue

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sgx-labs/same-telegram/internal/msgbox"
)

// TasksBaseDir returns the base directory for all task state directories.
func TasksBaseDir() string {
	return filepath.Join(msgbox.CompanyHQDir(), "tasks")
}

// StatePath returns the full path to a given state's directory.
func StatePath(state string) string {
	return filepath.Join(TasksBaseDir(), StateDirName(state))
}

// LogsDir returns the path to the task logs directory.
func LogsDir() string {
	return filepath.Join(TasksBaseDir(), "logs")
}

// allDirs returns all directories that need to exist.
func allDirs() []string {
	dirs := make([]string, 0, len(AllStates)+1)
	for _, s := range AllStates {
		dirs = append(dirs, StatePath(s))
	}
	dirs = append(dirs, LogsDir())
	return dirs
}

// EnsureDirs creates all task state directories under company-hq/tasks/.
func EnsureDirs() error {
	for _, dir := range allDirs() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create task dir %s: %w", dir, err)
		}
	}
	return nil
}

// Create writes a new task JSON file to the queue/ directory.
func Create(task *Task) error {
	if err := EnsureDirs(); err != nil {
		return err
	}

	data, err := task.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}

	path := filepath.Join(StatePath(StateQueued), task.Filename())
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write task file: %w", err)
	}
	return nil
}

// List returns all tasks in a given state directory.
func List(state string) ([]*Task, error) {
	dir := StatePath(state)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read task dir %s: %w", state, err)
	}

	var tasks []*Task
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		t, err := ParseTask(data)
		if err != nil {
			continue
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// ListAll returns tasks across all active states (queued, assigned, in_progress, review).
func ListAll() ([]*Task, error) {
	var all []*Task
	for _, state := range ActiveStates {
		tasks, err := List(state)
		if err != nil {
			return nil, err
		}
		all = append(all, tasks...)
	}
	return all, nil
}

// Move moves a task file from one state directory to another.
// It also updates the task's state and appends a history entry.
func Move(taskID, fromState, toState, by, note string) error {
	srcPath := filepath.Join(StatePath(fromState), taskID+".json")
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read task %s from %s: %w", taskID, fromState, err)
	}

	task, err := ParseTask(data)
	if err != nil {
		return err
	}

	if err := task.Transition(toState, by, note); err != nil {
		return err
	}

	// Write to destination
	if err := EnsureDirs(); err != nil {
		return err
	}

	newData, err := task.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}

	dstPath := filepath.Join(StatePath(toState), task.Filename())
	if err := os.WriteFile(dstPath, newData, 0o644); err != nil {
		return fmt.Errorf("write task to %s: %w", toState, err)
	}

	// Remove source file
	if err := os.Remove(srcPath); err != nil {
		return fmt.Errorf("remove task from %s: %w", fromState, err)
	}

	return nil
}

// Read finds and reads a task by ID, searching all state directories.
// Returns the task and its current state directory name.
func Read(taskID string) (*Task, string, error) {
	filename := taskID + ".json"
	for _, state := range AllStates {
		path := filepath.Join(StatePath(state), filename)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, "", fmt.Errorf("read task %s: %w", taskID, err)
		}
		t, err := ParseTask(data)
		if err != nil {
			return nil, "", err
		}
		return t, state, nil
	}
	return nil, "", fmt.Errorf("task %s not found", taskID)
}
