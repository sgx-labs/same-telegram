package daemon

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"github.com/sgx-labs/same-telegram/internal/config"
	"github.com/sgx-labs/same-telegram/internal/notify"
)

const maxMessageSize = 1024 * 1024 // 1 MB

// SocketServer listens for notifications from hook commands via unix socket.
type SocketServer struct {
	listener net.Listener
	logger   *log.Logger
	handler  func(*notify.Notification)
}

// NewSocketServer creates and starts listening on the unix socket.
func NewSocketServer(logger *log.Logger, handler func(*notify.Notification)) (*SocketServer, error) {
	socketPath := config.SocketPath()

	// Remove stale socket file if it exists
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale socket: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", socketPath, err)
	}

	// Restrict socket permissions
	if err := os.Chmod(socketPath, 0o600); err != nil {
		listener.Close()
		return nil, fmt.Errorf("chmod socket: %w", err)
	}

	logger.Printf("Socket listening on %s", socketPath)

	return &SocketServer{
		listener: listener,
		logger:   logger,
		handler:  handler,
	}, nil
}

// Serve accepts connections in a loop. Blocks until listener is closed.
func (s *SocketServer) Serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// Expected when listener is closed during shutdown
			return
		}
		go s.handleConn(conn)
	}
}

// Close stops the socket server and removes the socket file.
func (s *SocketServer) Close() error {
	socketPath := config.SocketPath()
	err := s.listener.Close()
	os.Remove(socketPath)
	return err
}

func (s *SocketServer) handleConn(conn net.Conn) {
	defer conn.Close()

	// Read length-prefixed message: 4-byte big-endian length + JSON payload
	var length uint32
	if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
		if err != io.EOF {
			s.logger.Printf("Read header error: %v", err)
		}
		s.writeResponse(conn, false, "read header failed")
		return
	}

	if length > maxMessageSize {
		s.logger.Printf("Message too large: %d bytes", length)
		s.writeResponse(conn, false, "message too large")
		return
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		s.logger.Printf("Read payload error: %v", err)
		s.writeResponse(conn, false, "read payload failed")
		return
	}

	var n notify.Notification
	if err := json.Unmarshal(data, &n); err != nil {
		s.logger.Printf("Unmarshal error: %v", err)
		s.writeResponse(conn, false, "invalid JSON")
		return
	}

	s.handler(&n)
	s.writeResponse(conn, true, "")
}

func (s *SocketServer) writeResponse(conn net.Conn, ok bool, errMsg string) {
	resp := notify.Response{OK: ok, Error: errMsg}
	json.NewEncoder(conn).Encode(resp)
}
