package bot

import (
	"context"
	"sync"
	"time"
)

const sessionTTL = 30 * time.Minute

// sessionEntry holds a Claude session ID with an expiry timestamp.
type sessionEntry struct {
	sessionID string
	expiresAt time.Time
}

// sessionStore tracks per-user Claude session IDs with a TTL.
// Sessions expire after sessionTTL of inactivity.
type sessionStore struct {
	mu       sync.RWMutex
	sessions map[int64]sessionEntry
}

func newSessionStore() *sessionStore {
	return &sessionStore{
		sessions: make(map[int64]sessionEntry),
	}
}

// Get returns the session ID for a user, or "" if expired/missing.
func (s *sessionStore) Get(userID int64) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.sessions[userID]
	if !ok {
		return ""
	}
	if time.Now().After(entry.expiresAt) {
		return ""
	}
	return entry.sessionID
}

// Set stores a session ID for a user, resetting the TTL.
func (s *sessionStore) Set(userID int64, sessionID string) {
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	s.sessions[userID] = sessionEntry{
		sessionID: sessionID,
		expiresAt: time.Now().Add(sessionTTL),
	}
	s.mu.Unlock()
}

// Clear removes the session ID for a user.
func (s *sessionStore) Clear(userID int64) {
	s.mu.Lock()
	delete(s.sessions, userID)
	s.mu.Unlock()
}

// StartEviction runs a background goroutine that removes expired entries every 5 minutes.
func (s *sessionStore) StartEviction(ctx context.Context) {
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

// evictExpired removes all expired session entries.
func (s *sessionStore) evictExpired() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for uid, entry := range s.sessions {
		if now.After(entry.expiresAt) {
			delete(s.sessions, uid)
		}
	}
}
