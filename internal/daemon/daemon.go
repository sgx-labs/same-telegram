package daemon

import (
	"context"
	"log"
	"os"

	"github.com/sgx-labs/same-telegram/internal/bot"
	"github.com/sgx-labs/same-telegram/internal/config"
	"github.com/sgx-labs/same-telegram/internal/notify"
)

// Daemon manages the Telegram bot and socket server.
type Daemon struct {
	cfg    *config.Config
	bot    *bot.Bot
	socket *SocketServer
	logger *log.Logger
}

// New creates a new Daemon instance.
func New(cfg *config.Config) (*Daemon, error) {
	logFile, err := os.OpenFile(config.LogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	logger := log.New(logFile, "same-telegram: ", log.LstdFlags|log.Lshortfile)

	tgBot, err := bot.New(cfg, logger)
	if err != nil {
		return nil, err
	}

	d := &Daemon{
		cfg:    cfg,
		bot:    tgBot,
		logger: logger,
	}

	socket, err := NewSocketServer(logger, d.onNotification)
	if err != nil {
		return nil, err
	}
	d.socket = socket

	return d, nil
}

// Run starts the daemon. Blocks until context is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	d.logger.Println("Daemon starting")

	// Start socket server in background
	go d.socket.Serve()

	// Run bot polling (blocks until ctx cancelled)
	err := d.bot.Run(ctx)

	// Cleanup
	d.socket.Close()
	d.logger.Println("Daemon stopped")
	return err
}

// onNotification handles incoming notifications from hook commands.
func (d *Daemon) onNotification(n *notify.Notification) {
	d.logger.Printf("Received notification: type=%s session=%s", n.Type, n.SessionID)

	// Check if this notification type is enabled
	switch n.Type {
	case notify.TypeSessionEnd:
		if !d.cfg.Notify.SessionEnd {
			return
		}
	case notify.TypeDecision:
		if !d.cfg.Notify.Decisions {
			return
		}
	case notify.TypeHandoff:
		if !d.cfg.Notify.Handoffs {
			return
		}
	}

	d.bot.SendNotification(n)
}
