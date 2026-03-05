package msgbox

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const pollInterval = 2 * time.Second

// Handler is called when a new agent message is found in the outbound directory.
type Handler func(msg *Message, filename string)

// Watcher polls the outbound directory for new agent messages.
type Watcher struct {
	logger  *log.Logger
	handler Handler
}

// NewWatcher creates a new outbound message watcher.
func NewWatcher(logger *log.Logger, handler Handler) *Watcher {
	return &Watcher{
		logger:  logger,
		handler: handler,
	}
}

// Watch polls the outbound directory until the context is cancelled.
func (w *Watcher) Watch(ctx context.Context) {
	if err := EnsureDirs(); err != nil {
		w.logger.Printf("msgbox: failed to create directories: %v", err)
		return
	}

	dir := OutboundDir()
	w.logger.Printf("msgbox: watching %s", dir)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.scanOnce(dir)
		}
	}
}

func (w *Watcher) scanOnce(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}

		path := filepath.Join(dir, name)
		msg, err := ReadMessage(path)
		if err != nil {
			w.logger.Printf("msgbox: skip invalid message %s: %v", name, err)
			// Move bad files so they don't block the queue
			ArchiveMessage(path)
			continue
		}

		w.handler(msg, name)

		if err := ArchiveMessage(path); err != nil {
			w.logger.Printf("msgbox: failed to archive %s: %v", name, err)
		}
	}
}

// ScanOnce is exported for testing — processes all pending messages once.
func (w *Watcher) ScanOnce() {
	w.scanOnce(OutboundDir())
}
