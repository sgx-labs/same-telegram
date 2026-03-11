package workspace

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"syscall"

	"github.com/creack/pty/v2"
	"github.com/gorilla/websocket"
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
		if token != s.AuthToken {
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

	// --- Session ---
	sess, err := s.getOrCreateSession()
	if err != nil {
		log.Printf("session error: %v", err)
		sendError(conn, "Your workspace is starting up. Please try again in a moment.")
		return
	}

	// --- PTY ---
	// Attach to tmux via a pseudo-terminal. Each WebSocket connection gets its
	// own PTY attachment to the shared tmux session. When the WebSocket closes,
	// we detach (SIGHUP) — tmux and the processes inside it keep running.
	cmd := sess.AttachCommand()
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("pty start failed: %v", err)
		sendError(conn, "Could not connect to your terminal session. It may still be initializing.")
		return
	}
	defer func() {
		ptmx.Close()
		// Detach from tmux, don't kill it. The session persists.
		if cmd.Process != nil {
			cmd.Process.Signal(syscall.SIGHUP)
		}
	}()

	log.Printf("client connected to session %q (pid %d)", sess.ID, cmd.Process.Pid)

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
	log.Printf("client disconnected from session %q (session still running)", sess.ID)
}

// sendError sends a human-readable error to the client as a JSON text message.
func sendError(conn *websocket.Conn, msg string) {
	payload, _ := json.Marshal(map[string]string{"error": msg})
	conn.WriteMessage(websocket.TextMessage, payload)
}
