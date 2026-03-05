package bot

import (
	"testing"
	"time"
)

func TestSessionStoreGetMissing(t *testing.T) {
	s := newSessionStore()
	if got := s.Get(123); got != "" {
		t.Errorf("Get on missing user should return empty, got %q", got)
	}
}

func TestSessionStoreSetAndGet(t *testing.T) {
	s := newSessionStore()
	s.Set(123, "session-abc-123")
	if got := s.Get(123); got != "session-abc-123" {
		t.Errorf("Get should return stored session, got %q", got)
	}
}

func TestSessionStoreSetEmpty(t *testing.T) {
	s := newSessionStore()
	s.Set(123, "") // should be a no-op
	if got := s.Get(123); got != "" {
		t.Errorf("Set with empty string should be no-op, got %q", got)
	}
}

func TestSessionStoreClear(t *testing.T) {
	s := newSessionStore()
	s.Set(123, "session-abc-123")
	s.Clear(123)
	if got := s.Get(123); got != "" {
		t.Errorf("Get after Clear should return empty, got %q", got)
	}
}

func TestSessionStoreClearMissing(t *testing.T) {
	s := newSessionStore()
	// Should not panic
	s.Clear(999)
}

func TestSessionStorePerUser(t *testing.T) {
	s := newSessionStore()
	s.Set(1, "session-one")
	s.Set(2, "session-two")

	if got := s.Get(1); got != "session-one" {
		t.Errorf("User 1 session should be session-one, got %q", got)
	}
	if got := s.Get(2); got != "session-two" {
		t.Errorf("User 2 session should be session-two, got %q", got)
	}

	s.Clear(1)
	if got := s.Get(1); got != "" {
		t.Errorf("User 1 should be cleared, got %q", got)
	}
	if got := s.Get(2); got != "session-two" {
		t.Errorf("User 2 should still have session, got %q", got)
	}
}

func TestSessionStoreOverwrite(t *testing.T) {
	s := newSessionStore()
	s.Set(1, "first")
	s.Set(1, "second")
	if got := s.Get(1); got != "second" {
		t.Errorf("Should return latest session, got %q", got)
	}
}

func TestSessionStoreExpiry(t *testing.T) {
	s := newSessionStore()
	// Manually insert an expired entry
	s.mu.Lock()
	s.sessions[123] = sessionEntry{
		sessionID: "expired-session",
		expiresAt: time.Now().Add(-1 * time.Minute),
	}
	s.mu.Unlock()

	if got := s.Get(123); got != "" {
		t.Errorf("Expired session should return empty, got %q", got)
	}
}

func TestSessionStoreTTLRefresh(t *testing.T) {
	s := newSessionStore()
	s.Set(1, "session-1")

	// Get the expiry
	s.mu.RLock()
	firstExpiry := s.sessions[1].expiresAt
	s.mu.RUnlock()

	// Set again (should refresh TTL)
	time.Sleep(1 * time.Millisecond)
	s.Set(1, "session-1")

	s.mu.RLock()
	secondExpiry := s.sessions[1].expiresAt
	s.mu.RUnlock()

	if !secondExpiry.After(firstExpiry) {
		t.Error("Setting again should refresh the TTL")
	}
}

func TestSessionStoreConcurrency(t *testing.T) {
	s := newSessionStore()
	done := make(chan struct{})

	for i := 0; i < 50; i++ {
		go func(id int64) {
			s.Set(id, "session")
			_ = s.Get(id)
			s.Clear(id)
			done <- struct{}{}
		}(int64(i))
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}
