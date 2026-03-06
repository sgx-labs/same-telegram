package bot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sgx-labs/same-telegram/internal/ai"
)

const (
	// maxConversationPairs is the maximum number of user+assistant message pairs to keep.
	maxConversationPairs = 10
	// maxConversationBytes is the soft limit on total conversation history size.
	maxConversationBytes = 8192
	// conversationTTL is how long a conversation stays active without new messages.
	conversationTTL = 30 * time.Minute
)

// conversationMessage represents a single message in a conversation.
type conversationMessage struct {
	Role    string // "user" or "assistant"
	Content string
}

// conversationEntry holds a user's conversation history with an expiry.
type conversationEntry struct {
	messages  []conversationMessage
	expiresAt time.Time
}

// conversationStore tracks per-user conversation history for API-based AI mode.
type conversationStore struct {
	mu            sync.RWMutex
	conversations map[int64]*conversationEntry
}

func newConversationStore() *conversationStore {
	return &conversationStore{
		conversations: make(map[int64]*conversationEntry),
	}
}

// Get returns the conversation history for a user, or nil if expired/missing.
func (s *conversationStore) Get(userID int64) []conversationMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.conversations[userID]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}
	// Return a copy to avoid races
	msgs := make([]conversationMessage, len(entry.messages))
	copy(msgs, entry.messages)
	return msgs
}

// Add appends a user message and assistant response to the conversation history.
// It enforces the sliding window: at most maxConversationPairs pairs, and
// trims from the front if the total text exceeds maxConversationBytes.
func (s *conversationStore) Add(userID int64, userMsg, assistantMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.conversations[userID]
	if !ok || time.Now().After(entry.expiresAt) {
		entry = &conversationEntry{}
		s.conversations[userID] = entry
	}

	entry.messages = append(entry.messages,
		conversationMessage{Role: "user", Content: userMsg},
		conversationMessage{Role: "assistant", Content: ai.StripThinkingTokens(assistantMsg)},
	)
	entry.expiresAt = time.Now().Add(conversationTTL)

	// Enforce max pairs (each pair = 2 messages)
	maxMsgs := maxConversationPairs * 2
	if len(entry.messages) > maxMsgs {
		entry.messages = entry.messages[len(entry.messages)-maxMsgs:]
	}

	// Enforce max bytes by trimming oldest pairs
	for len(entry.messages) > 2 && s.totalBytes(entry) > maxConversationBytes {
		entry.messages = entry.messages[2:] // drop oldest pair
	}
}

// Clear removes the conversation history for a user.
func (s *conversationStore) Clear(userID int64) {
	s.mu.Lock()
	delete(s.conversations, userID)
	s.mu.Unlock()
}

// StartEviction runs a background goroutine that removes expired entries every 5 minutes.
func (s *conversationStore) StartEviction(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.evictExpired()
			}
		}
	}()
}

// evictExpired removes all expired conversation entries.
func (s *conversationStore) evictExpired() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for uid, entry := range s.conversations {
		if now.After(entry.expiresAt) {
			delete(s.conversations, uid)
		}
	}
}

// totalBytes returns the total byte size of all messages in an entry.
// Caller must hold the lock.
func (s *conversationStore) totalBytes(entry *conversationEntry) int {
	total := 0
	for _, m := range entry.messages {
		total += len(m.Content)
	}
	return total
}

// BuildPromptWithHistory constructs a prompt string that includes conversation
// history as context, suitable for passing to a single-turn Chat API call.
// Returns just the prompt if there is no history.
//
// Uses XML-style tags as message delimiters to resist prompt injection
// (users cannot trivially fake "Assistant:" lines to manipulate context).
func BuildPromptWithHistory(history []conversationMessage, prompt string) string {
	if len(history) == 0 {
		return prompt
	}

	var sb strings.Builder
	sb.WriteString("Previous conversation:\n")
	for _, m := range history {
		if m.Role == "assistant" {
			sb.WriteString(fmt.Sprintf("<assistant_message>%s</assistant_message>\n\n", m.Content))
		} else {
			sb.WriteString(fmt.Sprintf("<user_message>%s</user_message>\n\n", m.Content))
		}
	}
	sb.WriteString("---\n\n<user_message>")
	sb.WriteString(prompt)
	sb.WriteString("</user_message>")
	return sb.String()
}
