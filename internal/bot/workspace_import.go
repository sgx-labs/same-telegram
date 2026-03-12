package bot

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/analytics"
)

// handleImportCommand processes /import — prepares to receive a file upload or imports from a URL.
//
// Usage:
//
//	/import                          — wait for file upload (up to 20MB)
//	/import https://example.com/f.tar.gz — download directly in workspace (no size limit)
func (b *Bot) handleImportCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	userID := msg.From.ID

	if b.orchestrator == nil {
		b.sendMarkdown(chatID, "Workspace mode is not enabled.")
		return
	}

	// Check the user has a workspace.
	userIDStr := strconv.FormatInt(userID, 10)
	um, err := b.orchestrator.Status(context.Background(), userIDStr)
	if err != nil || um == nil {
		b.sendMarkdown(chatID, "You don't have a workspace yet. Use /start to create one.")
		return
	}

	args := strings.TrimSpace(msg.CommandArguments())

	// If a URL argument was provided, import directly from URL.
	if args != "" {
		b.handleImportFromURL(chatID, userID, userIDStr, args)
		return
	}

	// No args — set pending import state and wait for file upload.
	b.onboarding.setPendingImport(userID)

	b.sendMarkdown(chatID,
		"*Import into workspace*\n\n"+
			"Send me a `.tar.gz` or `.zip` file (up to 20MB) and I'll extract it into your workspace.\n\n"+
			"For larger files, pass a URL:\n"+
			"`/import https://example.com/file.tar.gz`\n\n"+
			"The workspace downloads directly — no size limit.\n\n"+
			"_Send your file now, or /cancel to abort._")
}

// validateImportURL checks that the URL is safe for use in a shell command.
// Only http:// and https:// schemes are allowed. Returns the parsed URL or an error.
func validateImportURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow http and https schemes.
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme %q — only http:// and https:// are allowed", u.Scheme)
	}

	// Reject URLs with suspicious shell characters that could enable injection.
	// The URL will be single-quoted in the shell command, so single quotes are dangerous.
	// Also reject backticks, semicolons, pipes, $, and newlines.
	for _, ch := range rawURL {
		switch ch {
		case '\'', '`', ';', '|', '$', '\n', '\r':
			return nil, fmt.Errorf("URL contains invalid character %q", ch)
		}
	}

	if u.Host == "" {
		return nil, fmt.Errorf("URL has no host")
	}

	return u, nil
}

// handleImportFromURL downloads a file from a URL directly into the workspace.
func (b *Bot) handleImportFromURL(chatID, userID int64, userIDStr, rawURL string) {
	u, err := validateImportURL(rawURL)
	if err != nil {
		b.sendMarkdown(chatID, fmt.Sprintf("Invalid URL: %v", err))
		return
	}

	// Detect file type from URL path extension.
	lowerPath := strings.ToLower(u.Path)
	isTarGz := strings.HasSuffix(lowerPath, ".tar.gz") || strings.HasSuffix(lowerPath, ".tgz")
	isZip := strings.HasSuffix(lowerPath, ".zip")

	var script string
	var typeLabel string

	switch {
	case isTarGz:
		script = fmt.Sprintf(`curl -fsSL '%s' | tar xzf - -C /data`, rawURL)
		typeLabel = "tar.gz"
	case isZip:
		script = fmt.Sprintf(`curl -fsSL '%s' -o /tmp/import.zip && cd /data && unzip -o /tmp/import.zip && rm /tmp/import.zip`, rawURL)
		typeLabel = "zip"
	default:
		// No recognizable extension — try tar first (more common for workspace archives).
		script = fmt.Sprintf(`curl -fsSL '%s' | tar xzf - -C /data`, rawURL)
		typeLabel = "archive (assuming tar.gz)"
	}

	b.sendMarkdown(chatID, fmt.Sprintf("Downloading %s from URL into workspace...", typeLabel))

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		cmd := []string{"bash", "-c", script}
		if err := b.orchestrator.ExecInWorkspace(ctx, userIDStr, cmd); err != nil {
			b.logger.Printf("URL import failed for user %d: %v", userID, err)
			b.sendMarkdown(chatID, fmt.Sprintf("Import failed: %v\n\nMake sure the URL is publicly accessible and the file is a valid archive.", err))
			return
		}

		b.sendMarkdown(chatID, "Imported from URL into `/data` successfully.")
		b.logEvent(userID, analytics.EventWorkspaceImported, rawURL)
	}()
}

// handleImportFile processes a document upload when import is pending.
// Returns true if the message was handled.
func (b *Bot) handleImportFile(msg *tgbotapi.Message) bool {
	userID := msg.From.ID
	chatID := msg.Chat.ID

	// Check if this user has a pending import.
	if !b.onboarding.hasPendingImport(userID) {
		return false
	}

	// Clear the pending import immediately (one-shot).
	b.onboarding.clearPendingImport(userID)

	doc := msg.Document
	if doc == nil {
		b.sendMarkdown(chatID, "That doesn't look like a file. Send a `.tar.gz` or `.zip` document, or /cancel to abort.")
		// Re-set pending import so they can try again.
		b.onboarding.setPendingImport(userID)
		return true
	}

	filename := doc.FileName
	if filename == "" {
		filename = "unknown"
	}

	// Validate file type.
	isTarGz := strings.HasSuffix(strings.ToLower(filename), ".tar.gz") ||
		strings.HasSuffix(strings.ToLower(filename), ".tgz")
	isZip := strings.HasSuffix(strings.ToLower(filename), ".zip")

	if !isTarGz && !isZip {
		b.sendMarkdown(chatID,
			fmt.Sprintf("Unsupported file type: `%s`\n\nPlease send a `.tar.gz` or `.zip` file.", escapeMarkdown(filename)))
		// Re-set so they can try again with the correct file.
		b.onboarding.setPendingImport(userID)
		return true
	}

	b.sendMarkdown(chatID, fmt.Sprintf("Importing `%s` into your workspace...", escapeMarkdown(filename)))

	go func() {
		if err := b.importFileIntoWorkspace(chatID, userID, doc.FileID, filename, isTarGz); err != nil {
			b.logger.Printf("import failed for user %d: %v", userID, err)
			b.sendMarkdown(chatID, fmt.Sprintf("Import failed: %v", err))
			return
		}

		b.sendMarkdown(chatID, fmt.Sprintf("Imported `%s` into `/data` successfully.", escapeMarkdown(filename)))
		b.logEvent(userID, analytics.EventWorkspaceImported, filename)
	}()

	return true
}

// importFileIntoWorkspace downloads a file from Telegram and extracts it in the workspace.
func (b *Bot) importFileIntoWorkspace(chatID, userID int64, fileID, filename string, isTarGz bool) error {
	// Get the direct download URL from Telegram.
	fileURL, err := b.api.GetFileDirectURL(fileID)
	if err != nil {
		return fmt.Errorf("could not get file URL: %w", err)
	}

	userIDStr := strconv.FormatInt(userID, 10)

	// Build the extraction command.
	// The workspace container downloads the file directly from Telegram's servers
	// using curl, then pipes it into the appropriate extraction tool.
	// This avoids base64 encoding/shell argument size limits entirely.
	var script string
	if isTarGz {
		// curl downloads the file and pipes directly to tar for extraction.
		script = fmt.Sprintf(
			`curl -sfL '%s' | tar xzf - -C /data`,
			fileURL,
		)
	} else {
		// For zip: download to a temp file, extract, clean up.
		// unzip cannot read from stdin, so we need a temporary file.
		script = fmt.Sprintf(
			`curl -sfL '%s' -o /tmp/import.zip && cd /data && unzip -o /tmp/import.zip && rm -f /tmp/import.zip`,
			fileURL,
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := []string{"bash", "-c", script}
	if err := b.orchestrator.ExecInWorkspace(ctx, userIDStr, cmd); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	return nil
}
