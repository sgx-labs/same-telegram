package bot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/analytics"
)

// handleExportCommand processes /export — generates and sends a session summary.
//
// Usage:
//
//	/export        — export your own session summary
//	/export on     — enable auto-export on session end
//	/export off    — disable auto-export on session end
func (b *Bot) handleExportCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	userID := msg.From.ID
	userIDStr := strconv.FormatInt(userID, 10)

	if b.orchestrator == nil {
		b.sendMarkdown(chatID, "Workspace mode is not enabled.")
		return
	}

	args := strings.TrimSpace(msg.CommandArguments())

	switch args {
	case "on":
		b.setAutoExport(chatID, userID, userIDStr, true)
		return
	case "off":
		b.setAutoExport(chatID, userID, userIDStr, false)
		return
	case "takeout":
		b.handleTakeoutCommand(chatID, userID, userIDStr)
		return
	}

	// Default: generate and send a session summary.
	b.sendMarkdown(chatID, "Generating session summary...")

	go func() {
		summary, err := b.generateSessionSummary(userIDStr)
		if err != nil {
			b.logger.Printf("export failed for user %d: %v", userID, err)
			b.sendMarkdown(chatID, fmt.Sprintf("Could not generate session summary: %v", err))
			return
		}

		b.sendMarkdown(chatID, summary)
		b.logEvent(userID, analytics.EventSessionExported, "manual")
	}()
}

// setAutoExport toggles auto-export for a user by writing to their workspace config.
func (b *Bot) setAutoExport(chatID, userID int64, userIDStr string, enabled bool) {
	if b.orchestrator == nil {
		b.sendMarkdown(chatID, "Workspace mode is not enabled.")
		return
	}

	value := "false"
	if enabled {
		value = "true"
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Write the setting to the workspace's persistent config.
		// Use grep+sed to update existing value, or append if new.
		script := fmt.Sprintf(
			`if grep -q '^AUTO_EXPORT=' /data/.env 2>/dev/null; then `+
				`sed -i 's/^AUTO_EXPORT=.*/AUTO_EXPORT=%s/' /data/.env; `+
				`else echo 'AUTO_EXPORT=%s' >> /data/.env; fi && `+
				`tmux set-environment -g AUTO_EXPORT %s 2>/dev/null; true`,
			value, value, value,
		)

		cmd := []string{"bash", "-c", script}
		if err := b.orchestrator.ExecInWorkspace(ctx, userIDStr, cmd); err != nil {
			b.logger.Printf("failed to set auto-export for user %d: %v", userID, err)
			b.sendMarkdown(chatID, "Could not update auto-export setting. Is your workspace running?")
			return
		}

		if enabled {
			b.sendMarkdown(chatID, "Auto-export *enabled*. You'll receive a session summary when you close the terminal.")
		} else {
			b.sendMarkdown(chatID, "Auto-export *disabled*.")
		}
	}()
}

// generateSessionSummary builds a formatted session summary by exec-ing into the workspace.
func (b *Bot) generateSessionSummary(userIDStr string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Gather vault status and recent log in parallel.
	type result struct {
		name string
		data string
		err  error
	}
	ch := make(chan result, 2)

	// 1. Get vault status (memory count, topics, etc.)
	go func() {
		res, err := b.orchestrator.ExecInWorkspaceOutput(ctx, userIDStr,
			[]string{"bash", "-c", "same status --json 2>/dev/null || same status 2>/dev/null || echo '{}'"})
		if err != nil {
			ch <- result{name: "status", err: err}
			return
		}
		ch <- result{name: "status", data: res.Stdout}
	}()

	// 2. Get recent vault activity log.
	go func() {
		res, err := b.orchestrator.ExecInWorkspaceOutput(ctx, userIDStr,
			[]string{"bash", "-c", "same log --limit 20 --json 2>/dev/null || same log --limit 20 2>/dev/null || echo '[]'"})
		if err != nil {
			ch <- result{name: "log", err: err}
			return
		}
		ch <- result{name: "log", data: res.Stdout}
	}()

	var statusData, logData string
	for i := 0; i < 2; i++ {
		r := <-ch
		if r.err != nil {
			b.logger.Printf("export: %s exec failed for user %s: %v", r.name, userIDStr, r.err)
			continue
		}
		switch r.name {
		case "status":
			statusData = r.data
		case "log":
			logData = r.data
		}
	}

	return formatSessionSummary(statusData, logData), nil
}

// vaultStatus is the parsed JSON from `same status --json`.
type vaultStatus struct {
	MemoryCount int    `json:"memory_count"`
	TopicCount  int    `json:"topic_count"`
	VaultPath   string `json:"vault_path"`
	Model       string `json:"model"`
}

// vaultLogEntry is a single entry from `same log --json`.
type vaultLogEntry struct {
	Title     string `json:"title"`
	Type      string `json:"type"`
	Summary   string `json:"summary"`
	CreatedAt string `json:"created_at"`
}

// formatSessionSummary builds a Telegram-friendly session summary message.
func formatSessionSummary(statusJSON, logJSON string) string {
	var sb strings.Builder

	sb.WriteString("*Session Summary*\n\n")

	// Parse vault status.
	var status vaultStatus
	statusParsed := false
	if statusJSON != "" {
		if err := json.Unmarshal([]byte(strings.TrimSpace(statusJSON)), &status); err == nil && status.MemoryCount > 0 {
			statusParsed = true
		}
	}

	if statusParsed {
		sb.WriteString(fmt.Sprintf("*Vault:* %d memories", status.MemoryCount))
		if status.TopicCount > 0 {
			sb.WriteString(fmt.Sprintf(", %d topics", status.TopicCount))
		}
		sb.WriteString("\n")
		if status.Model != "" {
			sb.WriteString(fmt.Sprintf("*Model:* %s\n", escapeMarkdown(status.Model)))
		}
	} else if statusJSON != "" {
		// Could not parse JSON — show raw status (truncated).
		raw := strings.TrimSpace(statusJSON)
		if len(raw) > 200 {
			raw = raw[:200] + "..."
		}
		if raw != "" && raw != "{}" {
			sb.WriteString(fmt.Sprintf("*Vault status:*\n```\n%s\n```\n", raw))
		}
	}

	// Parse recent activity log.
	var logEntries []vaultLogEntry
	logParsed := false
	if logJSON != "" {
		trimmed := strings.TrimSpace(logJSON)
		if err := json.Unmarshal([]byte(trimmed), &logEntries); err == nil && len(logEntries) > 0 {
			logParsed = true
		}
	}

	if logParsed {
		sb.WriteString("\n*Recent activity:*\n")
		limit := 5
		if len(logEntries) < limit {
			limit = len(logEntries)
		}
		for i := 0; i < limit; i++ {
			entry := logEntries[i]
			label := entry.Title
			if label == "" {
				label = entry.Summary
			}
			if label == "" {
				label = entry.Type
			}
			if len(label) > 80 {
				label = label[:80] + "..."
			}
			sb.WriteString(fmt.Sprintf("  - %s\n", escapeMarkdown(label)))
		}
		if len(logEntries) > limit {
			sb.WriteString(fmt.Sprintf("  _...and %d more_\n", len(logEntries)-limit))
		}
	} else if logJSON != "" {
		// Could not parse JSON — show raw log (truncated).
		raw := strings.TrimSpace(logJSON)
		if len(raw) > 300 {
			raw = raw[:300] + "..."
		}
		if raw != "" && raw != "[]" {
			sb.WriteString(fmt.Sprintf("\n*Recent activity:*\n```\n%s\n```\n", raw))
		}
	}

	// If nothing was captured at all, show a generic message.
	if !statusParsed && !logParsed {
		sb.Reset()
		sb.WriteString("*Session Summary*\n\n")
		sb.WriteString("No vault activity recorded in this session.\n\n")
		sb.WriteString("_Tip: Use `same add` in the terminal to save memories to your vault._")
	}

	return sb.String()
}

// AutoExportForUser checks if auto-export is enabled for a user and sends
// a session summary to their chat. Called by the disconnect callback.
func (b *Bot) AutoExportForUser(userIDStr string, duration time.Duration) {
	if b.orchestrator == nil {
		return
	}

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		b.logger.Printf("auto-export: invalid user ID %q: %v", userIDStr, err)
		return
	}

	// Check if auto-export is enabled in the user's workspace.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := b.orchestrator.ExecInWorkspaceOutput(ctx, userIDStr,
		[]string{"bash", "-c", "grep -s '^AUTO_EXPORT=true' /data/.env && echo ENABLED || echo DISABLED"})
	if err != nil {
		b.logger.Printf("auto-export: could not check setting for user %s: %v", userIDStr, err)
		return
	}

	if !strings.Contains(res.Stdout, "ENABLED") {
		return
	}

	b.logger.Printf("auto-export: generating summary for user %s (session duration: %s)", userIDStr, duration.Round(time.Second))

	summary, err := b.generateSessionSummary(userIDStr)
	if err != nil {
		b.logger.Printf("auto-export: summary generation failed for user %s: %v", userIDStr, err)
		return
	}

	// Add session duration to the summary.
	durationStr := formatDuration(duration)
	summary += fmt.Sprintf("\n*Session:* %s", durationStr)

	// Send to the user's chat (user ID == chat ID for private chats in Telegram).
	b.sendMarkdown(userID, summary)
	b.logEvent(userID, analytics.EventSessionExported, "auto")
}

// handleTakeoutCommand creates a .tar.gz archive of the user's workspace and sends it as a Telegram document.
// For workspaces over 49MB, it splits the archive into chunks and sends them as separate documents.
func (b *Bot) handleTakeoutCommand(chatID, userID int64, userIDStr string) {
	b.sendMarkdown(chatID, "Preparing workspace takeout...")

	go func() {
		// Step 1: Create the archive and check its size.
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		createCmd := []string{"bash", "-c",
			`cd /data && tar czf /tmp/takeout.tar.gz --exclude='*.sock' --exclude='tmp/*' . 2>/dev/null && stat -c%s /tmp/takeout.tar.gz`,
		}
		res, err := b.orchestrator.ExecInWorkspaceOutput(ctx, userIDStr, createCmd)
		if err != nil {
			b.logger.Printf("takeout archive creation failed for user %d: %v", userID, err)
			b.sendMarkdown(chatID, fmt.Sprintf("Could not create takeout archive: %v", err))
			return
		}

		sizeStr := strings.TrimSpace(res.Stdout)
		archiveSize, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			b.logger.Printf("takeout could not parse archive size %q for user %d: %v", sizeStr, userID, err)
			b.sendMarkdown(chatID, "Could not determine archive size. Is your workspace running?")
			b.cleanupTakeout(userIDStr, false)
			return
		}

		if archiveSize == 0 {
			b.sendMarkdown(chatID, "Takeout archive is empty. Is your workspace running?")
			b.cleanupTakeout(userIDStr, false)
			return
		}

		const maxSingleFile = 49 * 1024 * 1024 // 49 MiB — margin below Telegram's 50MB limit

		if archiveSize <= int64(maxSingleFile) {
			// Small enough to send as a single file.
			b.sendSingleTakeout(ctx, chatID, userID, userIDStr, archiveSize)
		} else {
			// Too large — split and send in chunks.
			sizeMB := archiveSize / (1024 * 1024)
			b.sendChunkedTakeout(chatID, userID, userIDStr, archiveSize, sizeMB)
		}
	}()
}

// sendSingleTakeout handles the case where the archive fits in a single Telegram document.
func (b *Bot) sendSingleTakeout(ctx context.Context, chatID, userID int64, userIDStr string, archiveSize int64) {
	// Base64-encode the archive and download it.
	cmd := []string{"bash", "-c", "base64 -w0 /tmp/takeout.tar.gz"}
	res, err := b.orchestrator.ExecInWorkspaceOutput(ctx, userIDStr, cmd)
	if err != nil {
		b.logger.Printf("takeout base64 failed for user %d: %v", userID, err)
		b.sendMarkdown(chatID, fmt.Sprintf("Could not encode takeout archive: %v", err))
		b.cleanupTakeout(userIDStr, false)
		return
	}

	encoded := strings.TrimSpace(res.Stdout)
	if encoded == "" {
		b.sendMarkdown(chatID, "Takeout archive is empty.")
		b.cleanupTakeout(userIDStr, false)
		return
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		b.logger.Printf("takeout base64 decode failed for user %d: %v", userID, err)
		b.sendMarkdown(chatID, "Could not decode takeout archive. The workspace data may be corrupted.")
		b.cleanupTakeout(userIDStr, false)
		return
	}

	// Build a timestamped filename.
	filename := fmt.Sprintf("workspace-takeout-%s.tar.gz", time.Now().Format("2006-01-02"))
	sizeMB := float64(len(data)) / (1024 * 1024)
	caption := fmt.Sprintf("Workspace takeout (%.1fMB)", sizeMB)

	if err := b.sendDocument(chatID, filename, data, caption); err != nil {
		b.logger.Printf("takeout send failed for user %d: %v", userID, err)
		b.sendMarkdown(chatID, fmt.Sprintf("Could not send takeout file: %v", err))
		b.cleanupTakeout(userIDStr, false)
		return
	}

	b.cleanupTakeout(userIDStr, false)
	b.logEvent(userID, analytics.EventSessionExported, "takeout")
}

// sendChunkedTakeout splits a large archive into parts and sends each as a separate document.
func (b *Bot) sendChunkedTakeout(chatID, userID int64, userIDStr string, archiveSize, sizeMB int64) {
	// Split the archive into 45MB chunks.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	splitCmd := []string{"bash", "-c",
		`cd /tmp && split -b 45000000 takeout.tar.gz takeout-part- && ls -1 takeout-part-* | sort`,
	}
	res, err := b.orchestrator.ExecInWorkspaceOutput(ctx, userIDStr, splitCmd)
	if err != nil {
		b.logger.Printf("takeout split failed for user %d: %v", userID, err)
		b.sendMarkdown(chatID, fmt.Sprintf("Could not split takeout archive: %v", err))
		b.cleanupTakeout(userIDStr, true)
		return
	}

	// Parse part filenames.
	lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	var parts []string
	for _, line := range lines {
		part := strings.TrimSpace(line)
		if part != "" && strings.HasPrefix(part, "takeout-part-") {
			parts = append(parts, part)
		}
	}

	if len(parts) == 0 {
		b.sendMarkdown(chatID, "Could not split the archive into parts.")
		b.cleanupTakeout(userIDStr, true)
		return
	}

	totalParts := len(parts)
	b.sendMarkdown(chatID, fmt.Sprintf("Your workspace is %dMB — sending in %d parts...", sizeMB, totalParts))

	datestamp := time.Now().Format("2006-01-02")
	allSent := true

	for i, partFile := range parts {
		partNum := i + 1

		// Base64-encode this part.
		partCmd := []string{"bash", "-c", fmt.Sprintf("base64 -w0 /tmp/%s", partFile)}
		partRes, err := b.orchestrator.ExecInWorkspaceOutput(ctx, userIDStr, partCmd)
		if err != nil {
			b.logger.Printf("takeout part %d base64 failed for user %d: %v", partNum, userID, err)
			b.sendMarkdown(chatID, fmt.Sprintf("Failed to encode part %d/%d: %v", partNum, totalParts, err))
			allSent = false
			break
		}

		encoded := strings.TrimSpace(partRes.Stdout)
		if encoded == "" {
			b.sendMarkdown(chatID, fmt.Sprintf("Part %d/%d is empty — skipping.", partNum, totalParts))
			continue
		}

		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			b.logger.Printf("takeout part %d decode failed for user %d: %v", partNum, userID, err)
			b.sendMarkdown(chatID, fmt.Sprintf("Could not decode part %d/%d.", partNum, totalParts))
			allSent = false
			break
		}

		partSizeMB := float64(len(data)) / (1024 * 1024)
		filename := fmt.Sprintf("workspace-takeout-%s-%s", datestamp, partFile)
		caption := fmt.Sprintf("Part %d/%d (%.0fMB) — To reassemble: cat takeout-part-* > workspace.tar.gz", partNum, totalParts, partSizeMB)

		if err := b.sendDocument(chatID, filename, data, caption); err != nil {
			b.logger.Printf("takeout part %d send failed for user %d: %v", partNum, userID, err)
			b.sendMarkdown(chatID, fmt.Sprintf("Could not send part %d/%d: %v", partNum, totalParts, err))
			allSent = false
			break
		}
	}

	b.cleanupTakeout(userIDStr, true)

	if allSent {
		b.sendMarkdown(chatID, fmt.Sprintf(
			"Takeout complete! %d parts sent (%dMB total). To restore: `/import <file>`",
			totalParts, sizeMB))
		b.logEvent(userID, analytics.EventSessionExported, "takeout-chunked")
	}
}

// cleanupTakeout removes temporary takeout files from the workspace.
func (b *Bot) cleanupTakeout(userIDStr string, hasParts bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	script := "rm -f /tmp/takeout.tar.gz"
	if hasParts {
		script += " /tmp/takeout-part-*"
	}

	cmd := []string{"bash", "-c", script}
	_ = b.orchestrator.ExecInWorkspace(ctx, userIDStr, cmd)
}

// sendDocument sends a file as a Telegram document message.
func (b *Bot) sendDocument(chatID int64, filename string, data []byte, caption string) error {
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FileBytes{
		Name:  filename,
		Bytes: data,
	})
	doc.Caption = caption
	_, err := b.api.Send(doc)
	return err
}

// formatDuration is defined in team_commands.go — reused here.
