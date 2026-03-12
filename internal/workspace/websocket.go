package workspace

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/creack/pty/v2"
	"github.com/gorilla/websocket"

	"github.com/sgx-labs/same-telegram/internal/analytics"
)

const (
	// Ping interval for WebSocket keepalive.
	// Fly proxy kills idle connections after ~60s; 30s keeps them alive.
	pingInterval = 30 * time.Second
	// Pong timeout — if no pong received in this time, connection is dead.
	pongTimeout = 60 * time.Second
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Telegram Mini Apps run on telegram.org subdomains.
		// In production, validate against known origins.
		// For now, allow all — the auth token provides access control.
		return true
	},
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// --- Auth ---
	if s.AuthToken != "" {
		token := r.URL.Query().Get("token")
		// SECURITY: Use constant-time comparison to prevent timing-based
		// token guessing attacks.
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.AuthToken)) != 1 {
			http.Error(w, "unauthorized — check your workspace link", http.StatusUnauthorized)
			return
		}
	}

	// --- Upgrade ---
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// SECURITY: Limit incoming WebSocket message size to prevent memory exhaustion.
	// Terminal input messages are tiny (keystrokes); control messages (resize) are
	// small JSON. 64KB is generous while preventing abuse.
	conn.SetReadLimit(64 * 1024)

	// --- Session ---
	sess, err := s.getOrCreateSession()
	if err != nil {
		log.Printf("session error: %v", err)
		sendError(conn, "Your workspace is starting up. Please try again in a moment.")
		return
	}

	// --- PTY ---
	// Spawn a login shell directly on a pseudo-terminal. Each WebSocket
	// connection gets its own shell process. When the WebSocket closes,
	// we kill the shell and close the PTY.
	cmd := sess.AttachCommand()
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"HOME=/home/workspace",
		"USER=workspace",
		"SHELL=/bin/bash",
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("pty start failed: %v", err)
		sendError(conn, "Could not start your terminal session. Please try again in a moment.")
		return
	}
	defer func() {
		if cmd.Process != nil {
			// Kill the entire process group. pty.Start sets Setsid=true,
			// so the shell is a session leader and its PID == PGID.
			// Negative PID sends the signal to every process in the group,
			// ensuring child processes (e.g., claude, running builds) are
			// cleaned up when the WebSocket disconnects.
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		ptmx.Close()
	}()

	log.Printf("client connected to session %q (shell pid %d)", sess.ID, cmd.Process.Pid)

	// --- Analytics: track connection ---
	connectedAt := time.Now()
	if s.Analytics != nil {
		go s.Analytics.Track(0, analytics.EventWorkspaceConnected, map[string]string{
			"user_id": "0",
		})
	}

	// --- Keepalive ---
	// Set initial read deadline and pong handler to reset it on each pong.
	conn.SetReadDeadline(time.Now().Add(pongTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongTimeout))
		return nil
	})

	// --- I/O relay ---

	// PTY → WebSocket (terminal output to client).
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("terminal read error: %v", err)
				}
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
	}()

	// Server-side ping ticker — sends WebSocket ping frames to keep
	// the connection alive through Fly's proxy (~60s idle timeout).
	pingDone := make(chan struct{})
	go func() {
		defer close(pingDone)
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	// WebSocket → PTY (client input to terminal).
	go func() {
		for {
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Printf("client read error: %v", err)
				}
				ptmx.Close() // triggers the read goroutine to exit
				return
			}

			switch msgType {
			case websocket.BinaryMessage:
				// Raw terminal input (keystrokes).
				if _, err := ptmx.Write(msg); err != nil {
					log.Printf("terminal write error: %v", err)
					return
				}
			case websocket.TextMessage:
				// Control message (resize, future: theme, etc.).
				var ctrl controlMessage
				if err := json.Unmarshal(msg, &ctrl); err != nil {
					continue
				}
				switch ctrl.Type {
				case "resize":
					if ctrl.Cols > 0 && ctrl.Rows > 0 {
						pty.Setsize(ptmx, &pty.Winsize{
							Rows: ctrl.Rows,
							Cols: ctrl.Cols,
						})
					}
				}
			}
		}
	}()

	<-done
	<-pingDone

	// --- Analytics: track session duration ---
	duration := time.Since(connectedAt)
	durationSec := fmt.Sprintf("%.0f", duration.Seconds())
	log.Printf("client disconnected from session %q (duration: %s)", sess.ID, duration.Round(time.Second))
	if s.Analytics != nil {
		go func() {
			s.Analytics.Track(0, analytics.EventSessionDuration, map[string]string{
				"user_id":          "0",
				"duration_seconds": durationSec,
			})
			// Also log with value for simple aggregation.
			s.Analytics.Log(0, analytics.EventSessionDuration, durationSec)
		}()
	}

	// --- Disconnect callback ---
	if s.OnDisconnect != nil {
		go s.OnDisconnect(DisconnectInfo{
			SessionID: sess.ID,
			Duration:  duration,
		})
	}
}

// sendError sends a human-readable error to the client as a JSON text message.
func sendError(conn *websocket.Conn, msg string) {
	payload, _ := json.Marshal(map[string]string{"error": msg})
	conn.WriteMessage(websocket.TextMessage, payload)
}
