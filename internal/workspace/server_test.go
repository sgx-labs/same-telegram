package workspace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewServer_Defaults(t *testing.T) {
	s := NewServer("", "", "")
	if s.Addr != ":8080" {
		t.Errorf("expected default addr :8080, got %s", s.Addr)
	}
	if s.Shell != "bash" {
		t.Errorf("expected default shell bash, got %s", s.Shell)
	}
	if s.sessions == nil {
		t.Error("sessions map should be initialized")
	}
}

func TestNewServer_CustomValues(t *testing.T) {
	s := NewServer(":9090", "secret", "zsh")
	if s.Addr != ":9090" {
		t.Errorf("expected addr :9090, got %s", s.Addr)
	}
	if s.AuthToken != "secret" {
		t.Errorf("expected token secret, got %s", s.AuthToken)
	}
	if s.Shell != "zsh" {
		t.Errorf("expected shell zsh, got %s", s.Shell)
	}
}

func TestHealthEndpoint(t *testing.T) {
	s := NewServer(":0", "", "bash")

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
}

func TestServerShutdown(t *testing.T) {
	s := NewServer(":0", "", "bash")

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	// Give the server a moment to start.
	time.Sleep(50 * time.Millisecond)

	// Cancel should trigger graceful shutdown.
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected clean shutdown, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("server did not shut down within 3 seconds")
	}
}

func TestWebSocketAuth_Rejected(t *testing.T) {
	s := NewServer(":0", "correct-token", "bash")

	req := httptest.NewRequest("GET", "/ws?token=wrong-token", nil)
	w := httptest.NewRecorder()
	s.handleWebSocket(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong token, got %d", w.Code)
	}
}

func TestWebSocketAuth_NoTokenRequired(t *testing.T) {
	s := NewServer(":0", "", "bash")

	// With no auth token configured, the request should proceed past auth
	// (it will fail at WebSocket upgrade since this isn't a real WS request,
	// but it should NOT return 401).
	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()
	s.handleWebSocket(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Error("should not require auth when no token is configured")
	}
}
