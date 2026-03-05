package bot

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/taskqueue"
)

// cmdCreateTask parses a task description and creates a new task.
// Supports optional @role and priority:high/low annotations.
func cmdCreateTask(description string) (string, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		return "Usage: /task <description> [@role] [priority:high|low]", nil
	}

	var assignedTo string
	var priority string

	// Parse tokens for @role and priority:value
	words := strings.Fields(description)
	var titleWords []string

	for _, w := range words {
		switch {
		case strings.HasPrefix(w, "@"):
			assignedTo = strings.TrimPrefix(w, "@")
		case strings.HasPrefix(w, "priority:"):
			priority = strings.TrimPrefix(w, "priority:")
		default:
			titleWords = append(titleWords, w)
		}
	}

	title := strings.Join(titleWords, " ")
	// Strip surrounding quotes if present
	title = strings.Trim(title, "\"'")

	if title == "" {
		return "Task description cannot be empty.", nil
	}

	task := taskqueue.NewTask(title, title, assignedTo, priority, "ceo")

	if err := taskqueue.Create(task); err != nil {
		return "", fmt.Errorf("create task: %w", err)
	}

	reply := fmt.Sprintf("Task created: *%s*\nID: `%s`\nState: queued",
		escapeMarkdown(task.Title), task.ID)
	if assignedTo != "" {
		reply += fmt.Sprintf("\nAssigned: @%s", assignedTo)
	}
	if task.Priority != taskqueue.PriorityNormal {
		reply += fmt.Sprintf("\nPriority: %s", task.Priority)
	}
	return reply, nil
}

// cmdViewTask shows details for a specific task, resolved by number or ID.
func cmdViewTask(identifier string, tasks []*taskqueue.Task) (string, error) {
	task, idx, err := resolveTask(identifier, tasks)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("*Task #%d:* %s\n\n", idx+1, escapeMarkdown(task.Title)))
	b.WriteString(fmt.Sprintf("*State:* %s\n", task.State))
	if task.AssignedTo != "" {
		b.WriteString(fmt.Sprintf("*Agent:* %s\n", task.AssignedTo))
	}
	if task.Branch != "" {
		b.WriteString(fmt.Sprintf("*Branch:* `%s`\n", task.Branch))
	}
	b.WriteString(fmt.Sprintf("*Repo:* %s\n", task.Repo))
	b.WriteString(fmt.Sprintf("*Priority:* %s\n", task.Priority))
	b.WriteString(fmt.Sprintf("*Attempt:* %d/%d\n", task.Attempt, task.MaxAttempts))
	b.WriteString(fmt.Sprintf("*Tokens:* %d / %d\n", task.TokensUsed, task.TokenBudget))

	elapsed := task.ElapsedTime().Round(time.Second)
	b.WriteString(fmt.Sprintf("*Elapsed:* %s\n", elapsed))

	b.WriteString(fmt.Sprintf("\n*Created:* %s by %s\n",
		task.CreatedAt.Format("2006-01-02 15:04"), task.CreatedBy))

	if task.Error != "" {
		b.WriteString(fmt.Sprintf("*Error:* %s\n", escapeMarkdown(task.Error)))
	}
	if task.Feedback != "" {
		b.WriteString(fmt.Sprintf("*Feedback:* %s\n", escapeMarkdown(task.Feedback)))
	}

	b.WriteString(fmt.Sprintf("\n`/cancel-task %d` -- cancel this task", idx+1))
	return b.String(), nil
}

// cmdListTasks shows all active tasks with state, role, and elapsed time.
func cmdListTasks() (string, error) {
	tasks, err := taskqueue.ListAll()
	if err != nil {
		return "", fmt.Errorf("list tasks: %w", err)
	}

	if len(tasks) == 0 {
		return "No active tasks.", nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("*Active Tasks (%d)*\n\n", len(tasks)))

	for i, t := range tasks {
		elapsed := formatElapsed(t.ElapsedTime())
		role := t.AssignedTo
		if role == "" {
			role = "unassigned"
		}

		b.WriteString(fmt.Sprintf("*%d.* \\[%s\\] %s -- @%s (%s)\n",
			i+1, t.State, escapeMarkdown(t.Title), role, elapsed))
	}

	b.WriteString("\nUse `/task <n>` for details.")
	return b.String(), nil
}

// cmdCancelTask cancels a task by moving it to the failed state.
func cmdCancelTask(identifier string) (string, error) {
	tasks, err := taskqueue.ListAll()
	if err != nil {
		return "", fmt.Errorf("list tasks: %w", err)
	}

	task, idx, err := resolveTask(identifier, tasks)
	if err != nil {
		return "", err
	}

	// Move the task to failed
	err = taskqueue.Move(task.ID, task.State, taskqueue.StateFailed, "ceo", "Cancelled by CEO")
	if err != nil {
		return "", fmt.Errorf("cancel task: %w", err)
	}

	return fmt.Sprintf("Task #%d cancelled: *%s*", idx+1, escapeMarkdown(task.Title)), nil
}

// resolveTask finds a task by number (1-based) or task ID.
func resolveTask(identifier string, tasks []*taskqueue.Task) (*taskqueue.Task, int, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return nil, 0, fmt.Errorf("specify a task number or ID")
	}

	// Try as number first
	if num, err := strconv.Atoi(identifier); err == nil {
		if num < 1 || num > len(tasks) {
			return nil, 0, fmt.Errorf("task number %d out of range (1-%d)", num, len(tasks))
		}
		return tasks[num-1], num - 1, nil
	}

	// Try as task ID
	for i, t := range tasks {
		if t.ID == identifier {
			return t, i, nil
		}
	}

	return nil, 0, fmt.Errorf("no task matching %q", identifier)
}

// handleTaskCommand dispatches /task based on whether the argument looks
// like a number (view task) or text (create task).
func (b *Bot) handleTaskCommand(msg *tgbotapi.Message, args string) {
	args = strings.TrimSpace(args)
	if args == "" {
		b.sendMarkdown(msg.Chat.ID, "Usage:\n`/task <description>` -- create a task\n`/task <n>` -- view task details")
		return
	}

	// If the argument is a number, view that task
	if _, err := strconv.Atoi(args); err == nil {
		tasks, lerr := taskqueue.ListAll()
		if lerr != nil {
			b.sendMarkdown(msg.Chat.ID, fmt.Sprintf("Error: %s", lerr))
			return
		}
		reply, verr := cmdViewTask(args, tasks)
		if verr != nil {
			b.sendMarkdown(msg.Chat.ID, fmt.Sprintf("Error: %s", verr))
			return
		}
		b.sendMarkdown(msg.Chat.ID, reply)
		return
	}

	// Otherwise, create a new task
	reply, err := cmdCreateTask(args)
	if err != nil {
		b.sendMarkdown(msg.Chat.ID, fmt.Sprintf("Error: %s", err))
		return
	}
	b.sendMarkdown(msg.Chat.ID, reply)
}

// formatElapsed returns a human-readable elapsed time string.
func formatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}
