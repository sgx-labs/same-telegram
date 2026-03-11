package workspace

import (
	"fmt"
	"os/exec"
	"sync"
)

// Session represents a persistent tmux session. Multiple WebSocket clients
// can attach and detach without affecting the running process. This is what
// makes "close Telegram, come back later, everything is still there" work.
type Session struct {
	ID    string // tmux session name (e.g., "main")
	Shell string // shell command running inside tmux

	mu   sync.Mutex
	dead bool
}

// NewSession creates (or reattaches to) a tmux session.
//
// If the session already exists (e.g., after a server restart while tmux
// kept running), it reuses it rather than creating a duplicate. This means
// users don't lose their work even if the workspace server restarts.
func NewSession(id, shell string) (*Session, error) {
	if id == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}

	s := &Session{ID: id, Shell: shell}

	// Check if tmux session already exists (container restart scenario).
	check := exec.Command("tmux", "has-session", "-t", id)
	if err := check.Run(); err == nil {
		// Session exists — reuse it. No work lost.
		return s, nil
	}

	// Create new tmux session with a reasonable default size.
	// The client will send a resize event on connect to match their viewport.
	cmd := exec.Command("tmux", "new-session", "-d", "-s", id, "-x", "120", "-y", "40", shell)
	if out, err := cmd.CombinedOutput(); err != nil {
		hint := ""
		if isNotFound(err) {
			hint = " — is tmux installed? (apt install tmux)"
		}
		return nil, fmt.Errorf("could not start terminal session%s: %w: %s", hint, err, out)
	}

	return s, nil
}

// IsAlive checks if the tmux session is still running.
func (s *Session) IsAlive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dead {
		return false
	}

	check := exec.Command("tmux", "has-session", "-t", s.ID)
	if err := check.Run(); err != nil {
		s.dead = true
		return false
	}
	return true
}

// AttachCommand returns an exec.Cmd that attaches to this tmux session.
// The caller sets up a PTY on the returned command for I/O.
func (s *Session) AttachCommand() *exec.Cmd {
	return exec.Command("tmux", "attach-session", "-t", s.ID)
}

// isNotFound checks if an error is because the command wasn't found.
func isNotFound(err error) bool {
	if exitErr, ok := err.(*exec.Error); ok {
		return exitErr.Err == exec.ErrNotFound
	}
	return false
}
