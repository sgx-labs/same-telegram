// Package workspace provides a WebSocket-to-terminal relay server.
//
// It bridges xterm.js clients (Telegram Mini App, web browser, future native
// apps) to a persistent tmux session running inside the container. The tmux
// session survives client disconnects — users can close Telegram and come back
// hours later to find their work exactly where they left it.
//
// Design notes:
//   - One tmux session per container ("main"). Multiple clients can attach.
//   - WebSocket carries binary terminal I/O and JSON control messages (resize).
//   - Auth is token-based. In production, tokens are issued by the bot after
//     verifying Telegram identity. For development, --token flag or no auth.
//   - The server also serves the xterm.js frontend for direct browser access.
package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

// Server relays WebSocket connections to a persistent tmux session.
type Server struct {
	Addr      string // listen address (default ":8080")
	AuthToken string // required token for connections (empty = no auth)
	Shell     string // shell to run inside tmux (default "bash")

	mu       sync.Mutex
	sessions map[string]*Session
}

// NewServer creates a workspace server.
func NewServer(addr, authToken, shell string) *Server {
	if addr == "" {
		addr = ":8080"
	}
	if shell == "" {
		shell = "bash"
	}
	return &Server{
		Addr:      addr,
		AuthToken: authToken,
		Shell:     shell,
		sessions:  make(map[string]*Session),
	}
}

// controlMessage is a JSON message from the client for terminal control.
// Currently supports resize; extensible for future control commands.
type controlMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

// machineReplay is middleware for Fly.io multi-machine routing.
// When requests include an "instance" query parameter, it checks whether this
// machine is the intended target. If not, it responds with a fly-replay header
// so Fly's proxy retries the request on the correct machine.
func machineReplay(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		instance := r.URL.Query().Get("instance")
		if instance == "" {
			next.ServeHTTP(w, r)
			return
		}

		self := os.Getenv("FLY_MACHINE_ID")
		if self == "" {
			// Local dev — no machine ID, pass through.
			next.ServeHTTP(w, r)
			return
		}

		if instance != self {
			w.Header().Set("fly-replay", "instance="+instance)
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Run starts the HTTP server and blocks until ctx is cancelled.
// On shutdown, active sessions are preserved (tmux keeps running).
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)

	// Serve the xterm.js terminal UI if available.
	webDir := os.Getenv("WEB_DIR")
	if webDir == "" {
		webDir = "web/terminal"
	}
	if info, err := os.Stat(webDir); err == nil && info.IsDir() {
		mux.Handle("/", http.FileServer(http.Dir(webDir)))
	}

	srv := &http.Server{
		Addr:              s.Addr,
		Handler:           machineReplay(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("could not start workspace server on %s: %w (is another process using this port?)", s.Addr, err)
	}
	log.Printf("workspace server listening on %s", s.Addr)

	// Graceful shutdown: close listener, let in-flight requests finish.
	go func() {
		<-ctx.Done()
		log.Println("shutting down workspace server (sessions will persist in tmux)")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	if err := srv.Serve(ln); err != http.ErrServerClosed {
		return fmt.Errorf("workspace server stopped unexpectedly: %w", err)
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	sessionCount := len(s.sessions)
	alive := 0
	for _, sess := range s.sessions {
		if sess.IsAlive() {
			alive++
		}
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":   "ok",
		"sessions": sessionCount,
		"alive":    alive,
	})
}

// getOrCreateSession returns the default tmux session, creating it if needed.
// If the session died (e.g., user typed `exit`), a new one is created.
func (s *Server) getOrCreateSession() (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	const defaultID = "main"
	if sess, ok := s.sessions[defaultID]; ok && sess.IsAlive() {
		return sess, nil
	}

	sess, err := NewSession(defaultID, s.Shell)
	if err != nil {
		return nil, fmt.Errorf("could not create terminal session: %w", err)
	}
	s.sessions[defaultID] = sess
	return sess, nil
}
