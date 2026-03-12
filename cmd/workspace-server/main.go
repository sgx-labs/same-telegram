// workspace-server provides a WebSocket-to-terminal relay for per-user
// cloud workspaces. It manages a persistent tmux session and bridges
// xterm.js clients to it over WebSocket.
//
// Usage:
//
//	workspace-server [--addr :8080] [--token SECRET] [--shell bash]
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/sgx-labs/same-telegram/internal/analytics"
	"github.com/sgx-labs/same-telegram/internal/workspace"
)

func main() {
	addr := flag.String("addr", envOr("WORKSPACE_ADDR", ":8080"), "listen address")
	token := flag.String("token", os.Getenv("WORKSPACE_TOKEN"), "auth token for WebSocket connections")
	shell := flag.String("shell", envOr("WORKSPACE_SHELL", "bash"), "shell to run inside tmux")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv := workspace.NewServer(*addr, *token, *shell)

	// Initialize analytics store (non-fatal — server works without it).
	analyticsPath := envOr("ANALYTICS_DB", filepath.Join(os.TempDir(), "workspace-analytics.db"))
	if a, err := analytics.New(analyticsPath); err != nil {
		log.Printf("Analytics disabled: %v", err)
	} else {
		srv.Analytics = a
		log.Printf("Analytics enabled: %s", analyticsPath)
	}

	log.Printf("starting workspace server on %s (shell=%s)", *addr, *shell)
	if err := srv.Run(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
