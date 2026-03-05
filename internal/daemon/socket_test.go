package daemon

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"github.com/sgx-labs/same-telegram/internal/notify"
)

// shortSocketPath returns a short socket path in /tmp to avoid macOS 104-char Unix socket limit.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	// Use a short path under /tmp to stay well within the 104-char limit.
	path := fmt.Sprintf("/tmp/st-%d.sock", os.Getpid())
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func newTestServer(t *testing.T, handler func(*notify.Notification)) (*SocketServer, string) {
	t.Helper()
	socketPath := shortSocketPath(t)

	logger := log.New(os.Stderr, "test: ", 0)

	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	os.Chmod(socketPath, 0o600)

	srv := &SocketServer{
		listener: listener,
		logger:   logger,
		handler:  handler,
	}
	return srv, socketPath
}

func sendNotification(t *testing.T, socketPath string, n *notify.Notification) (*notify.Response, error) {
	t.Helper()
	conn, err := net.DialTimeout("unix", socketPath, time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	data, err := json.Marshal(n)
	if err != nil {
		return nil, err
	}

	length := uint32(len(data))
	if err := binary.Write(conn, binary.BigEndian, length); err != nil {
		return nil, err
	}
	if _, err := conn.Write(data); err != nil {
		return nil, err
	}

	var resp notify.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func TestSocketServerHandleValidNotification(t *testing.T) {
	received := make(chan *notify.Notification, 1)
	handler := func(n *notify.Notification) {
		received <- n
	}

	srv, socketPath := newTestServer(t, handler)
	go srv.Serve()
	defer srv.Close()

	time.Sleep(10 * time.Millisecond)

	n := &notify.Notification{
		Type:      notify.TypeSessionEnd,
		SessionID: "test-session",
		Summary:   "Test notification",
	}
	resp, err := sendNotification(t, socketPath, n)
	if err != nil {
		t.Fatalf("sendNotification: %v", err)
	}
	if !resp.OK {
		t.Errorf("Expected OK response, got error: %s", resp.Error)
	}

	select {
	case got := <-received:
		if got.SessionID != "test-session" {
			t.Errorf("SessionID = %q, want 'test-session'", got.SessionID)
		}
		if got.Type != notify.TypeSessionEnd {
			t.Errorf("Type = %q, want %q", got.Type, notify.TypeSessionEnd)
		}
	case <-time.After(time.Second):
		t.Error("Timed out waiting for notification handler to be called")
	}
}

func TestSocketServerRejectsOversizedMessage(t *testing.T) {
	handler := func(n *notify.Notification) {}

	srv, socketPath := newTestServer(t, handler)
	go srv.Serve()
	defer srv.Close()

	time.Sleep(10 * time.Millisecond)

	conn, err := net.DialTimeout("unix", socketPath, time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Send a length exceeding maxMessageSize
	oversized := uint32(maxMessageSize + 1)
	if err := binary.Write(conn, binary.BigEndian, oversized); err != nil {
		t.Fatalf("Write header: %v", err)
	}

	var resp notify.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp.OK {
		t.Error("Expected error response for oversized message")
	}
	if resp.Error == "" {
		t.Error("Expected error message in response")
	}
}

func TestSocketServerRejectsInvalidJSON(t *testing.T) {
	handler := func(n *notify.Notification) {}

	srv, socketPath := newTestServer(t, handler)
	go srv.Serve()
	defer srv.Close()

	time.Sleep(10 * time.Millisecond)

	conn, err := net.DialTimeout("unix", socketPath, time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Send valid length prefix but invalid JSON payload
	payload := []byte("this is not valid json{{{{")
	if err := binary.Write(conn, binary.BigEndian, uint32(len(payload))); err != nil {
		t.Fatalf("Write header: %v", err)
	}
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("Write payload: %v", err)
	}

	var resp notify.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp.OK {
		t.Error("Expected error response for invalid JSON")
	}
}

func TestSocketServerConcurrentConnections(t *testing.T) {
	done := make(chan struct{}, 5)
	handler := func(n *notify.Notification) {
		done <- struct{}{}
	}

	srv, socketPath := newTestServer(t, handler)
	go srv.Serve()
	defer srv.Close()

	time.Sleep(10 * time.Millisecond)

	n := &notify.Notification{Type: notify.TypeCustom, Summary: "concurrent test"}

	for i := 0; i < 5; i++ {
		go func() {
			sendNotification(t, socketPath, n)
		}()
	}

	timeout := time.After(3 * time.Second)
	for i := 0; i < 5; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Errorf("Timed out: only received %d of 5 notifications", i)
			return
		}
	}
}

func TestSocketServerClose(t *testing.T) {
	handler := func(n *notify.Notification) {}
	srv, _ := newTestServer(t, handler)

	if err := srv.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		srv.Serve()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Serve() did not return after Close()")
	}
}
