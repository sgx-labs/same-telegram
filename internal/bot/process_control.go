package bot

import (
	"context"
	"fmt"
	"sync/atomic"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/exec"
)

// inflightEntry tracks a single in-flight request's cancel func and generation.
type inflightEntry struct {
	cancel context.CancelFunc
	gen    uint64
}

// generation is a global counter to uniquely identify each inflight request.
var generation atomic.Uint64

// startInflight creates a cancellable context for a user's in-flight request.
// If a request is already in flight, it cancels it first.
// Returns the context and a cleanup function that MUST be deferred.
func (b *Bot) startInflight(userID int64) (context.Context, func()) {
	b.inflightMu.Lock()
	if entry, ok := b.inflight[userID]; ok {
		entry.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	gen := generation.Add(1)
	b.inflight[userID] = inflightEntry{cancel: cancel, gen: gen}
	b.inflightMu.Unlock()

	cleanup := func() {
		b.inflightMu.Lock()
		// Only delete if it's still our entry (not replaced by a newer request).
		if cur, ok := b.inflight[userID]; ok && cur.gen == gen {
			delete(b.inflight, userID)
		}
		b.inflightMu.Unlock()
	}
	return ctx, cleanup
}

// cancelInflight cancels any in-flight request for the given user.
// Returns true if a request was cancelled.
func (b *Bot) cancelInflight(userID int64) bool {
	b.inflightMu.Lock()
	defer b.inflightMu.Unlock()
	if entry, ok := b.inflight[userID]; ok {
		entry.cancel()
		delete(b.inflight, userID)
		return true
	}
	return false
}

// handleStop processes the /stop command.
func (b *Bot) handleStop(msg *tgbotapi.Message) {
	if b.cancelInflight(msg.From.ID) {
		b.sendMarkdown(msg.Chat.ID, "Stopped.")
	} else {
		b.sendMarkdown(msg.Chat.ID, "No request in progress.")
	}
}

// handleEditedMessage processes edited messages from the user.
// If an AI request is in flight, cancel it and resubmit.
// Otherwise, treat the edited message as a new message.
func (b *Bot) handleEditedMessage(msg *tgbotapi.Message) {
	if !b.allowedUsers[msg.From.ID] {
		b.logger.Printf("Dropped edited message from unauthorized user %d (%s)", msg.From.ID, msg.From.UserName)
		return
	}

	// Cancel any in-flight request for this user.
	b.cancelInflight(msg.From.ID)

	// Treat the edited message as a new message.
	b.handleUpdate(msg)
}

// dispatchCancellable runs a SAME CLI command in the background with cancellation support.
// It sends the result (or "Stopped.") to the chat.
func (b *Bot) dispatchCancellable(chatID int64, userID int64, fn func(ctx context.Context) (string, error)) {
	ctx, cleanup := b.startInflight(userID)

	go func() {
		defer cleanup()

		reply, err := fn(ctx)
		if ctx.Err() != nil {
			b.sendMarkdown(chatID, "Stopped.")
			return
		}
		if err != nil {
			b.sendMarkdown(chatID, fmt.Sprintf("Error: %s", err))
			return
		}
		b.sendMarkdown(chatID, reply)
	}()
}

// dispatchAsk runs /ask with cancellation support.
func (b *Bot) dispatchAsk(chatID int64, userID int64, question string) {
	b.dispatchCancellable(chatID, userID, func(ctx context.Context) (string, error) {
		out, err := exec.RunSameCtx(ctx, "ask", question)
		if err != nil {
			return "", err
		}
		if len(out) > 3500 {
			out = out[:3500] + "\n... (truncated)"
		}
		return fmt.Sprintf("*Answer*\n\n%s", out), nil
	})
}

// dispatchSearch runs search with cancellation support.
func (b *Bot) dispatchSearch(chatID int64, userID int64, query string) {
	b.dispatchCancellable(chatID, userID, func(ctx context.Context) (string, error) {
		out, err := exec.RunSameCtx(ctx, "search", "--json", query)
		if err != nil {
			return "", err
		}
		if len(out) > 3500 {
			out = out[:3500] + "\n... (truncated)"
		}
		return fmt.Sprintf("*Search: %s*\n\n```\n%s\n```", query, out), nil
	})
}
