package workspace

import (
	"fmt"
	"os/exec"
	"sync"
)

// Session represents a terminal session. Each WebSocket connection gets its
// own shell process via a PTY, giving clean line-by-line output without
// any terminal multiplexer escape sequences.
type Session struct {
	ID    string // session identifier (e.g., "main")
	Shell string // shell to run (e.g., "bash")

	mu   sync.Mutex
	dead bool
}

// NewSession creates a new session struct.
// Unlike the previous tmux-based approach, this just creates the tracking
// struct — the actual shell process is spawned per-connection in the
// WebSocket handler via AttachCommand().
func NewSession(id, shell string) (*Session, error) {
	if id == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}

	return &Session{ID: id, Shell: shell}, nil
}

// IsAlive returns whether this session is available for use.
// Since sessions are now lightweight structs (the real process lives
// per-connection), this always returns true unless explicitly marked dead.
func (s *Session) IsAlive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.dead
}

// AttachCommand returns an exec.Cmd for a login shell.
// The caller sets up a PTY on the returned command for I/O.
func (s *Session) AttachCommand() *exec.Cmd {
	return exec.Command(s.Shell, "-l")
}
