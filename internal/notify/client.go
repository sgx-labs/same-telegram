package notify

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/sgx-labs/same-telegram/internal/config"
)

// Send delivers a notification to the running daemon via unix socket.
// Returns nil if the daemon is not running (notification is silently dropped).
func Send(n *Notification) error {
	socketPath := config.SocketPath()

	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		// Daemon not running — silently drop notification.
		// This is expected behavior when the daemon isn't started.
		return nil
	}
	defer conn.Close()

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	data, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	// Write length-prefixed message: 4-byte big-endian length + JSON payload
	length := uint32(len(data))
	header := []byte{byte(length >> 24), byte(length >> 16), byte(length >> 8), byte(length)}
	if _, err := conn.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}

	// Read response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("daemon error: %s", resp.Error)
	}
	return nil
}
