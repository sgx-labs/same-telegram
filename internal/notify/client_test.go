package notify

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"testing"
	"time"
)

// startTestSocket starts a unix socket server that receives one notification
// and sends back a response. Returns the socket path and a channel with the received notification.
func startTestSocket(t *testing.T, resp Response) (string, <-chan *Notification) {
	t.Helper()
	socketPath := fmt.Sprintf("/tmp/st-notify-%d.sock", os.Getpid())
	t.Cleanup(func() { os.Remove(socketPath) })
	os.Remove(socketPath)

	received := make(chan *Notification, 1)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))

		// Read length-prefixed message
		var length uint32
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			return
		}
		data := make([]byte, length)
		if _, err := conn.Read(data); err != nil {
			return
		}

		var n Notification
		json.Unmarshal(data, &n)
		received <- &n

		// Write response
		json.NewEncoder(conn).Encode(resp)
	}()

	return socketPath, received
}

func TestSendNoDaemon(t *testing.T) {
	// When no daemon is running, Send should return nil (silent drop)
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	n := &Notification{
		Type:    TypeSessionEnd,
		Summary: "test",
	}
	err := Send(n)
	if err != nil {
		t.Errorf("Send with no daemon should return nil, got: %v", err)
	}
}

func TestSendToSocket(t *testing.T) {
	// Set up HOME to point to a temp dir with our test socket
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create .same directory and symlink the socket
	sameDir := fmt.Sprintf("%s/.same", tmpDir)
	os.MkdirAll(sameDir, 0o755)

	socketPath := fmt.Sprintf("%s/telegram.sock", sameDir)
	os.Remove(socketPath)

	// Start a real socket server at the expected path
	received := make(chan *Notification, 1)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))

		var length uint32
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			return
		}
		data := make([]byte, length)
		if _, err := conn.Read(data); err != nil {
			return
		}
		var n Notification
		json.Unmarshal(data, &n)
		received <- &n

		json.NewEncoder(conn).Encode(Response{OK: true})
	}()

	n := &Notification{
		Type:      TypeSessionEnd,
		SessionID: "test-send-123",
		Summary:   "Integration test",
		Timestamp: time.Now(),
	}

	err = Send(n)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case got := <-received:
		if got.Type != TypeSessionEnd {
			t.Errorf("Type = %q, want %q", got.Type, TypeSessionEnd)
		}
		if got.SessionID != "test-send-123" {
			t.Errorf("SessionID = %q, want test-send-123", got.SessionID)
		}
	case <-time.After(3 * time.Second):
		t.Error("Timed out waiting for notification")
	}
}

func TestSendDaemonError(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	sameDir := fmt.Sprintf("%s/.same", tmpDir)
	os.MkdirAll(sameDir, 0o755)
	socketPath := fmt.Sprintf("%s/telegram.sock", sameDir)

	// Start a server that returns an error response
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))

		var length uint32
		binary.Read(conn, binary.BigEndian, &length)
		data := make([]byte, length)
		conn.Read(data)

		json.NewEncoder(conn).Encode(Response{OK: false, Error: "test error from daemon"})
	}()

	n := &Notification{Type: TypeCustom, Summary: "test"}
	err = Send(n)
	if err == nil {
		t.Error("Expected error when daemon returns error response")
	}
	if err != nil && !contains(err.Error(), "test error from daemon") {
		t.Errorf("Error should contain daemon message, got: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
