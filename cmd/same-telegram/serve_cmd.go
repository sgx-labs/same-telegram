package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/sgx-labs/same-telegram/internal/config"
	"github.com/sgx-labs/same-telegram/internal/daemon"
)

var (
	serveForeground bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Telegram bot daemon",
	Long: `Starts the same-telegram daemon which runs the Telegram bot
and listens for hook notifications via unix socket.

By default, runs in the background. Use --fg for foreground mode.`,
	RunE: runServe,
}

var serveStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running daemon",
	RunE:  runServeStop,
}

var serveStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if daemon is running",
	RunE:  runServeStatus,
}

func init() {
	serveCmd.Flags().BoolVar(&serveForeground, "fg", false, "Run in foreground")
	serveCmd.AddCommand(serveStopCmd)
	serveCmd.AddCommand(serveStatusCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	// If not foreground and not re-exec child, launch background
	if !serveForeground && os.Getenv("_SAME_TELEGRAM_BG") == "" {
		return launchBackground()
	}

	// Foreground mode: run the daemon
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	d, err := daemon.New(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-ctx.Done():
		case <-sigCh:
			fmt.Fprintln(os.Stderr, "same-telegram: shutting down...")
			cancel()
		}
	}()

	// Write PID file
	writePID(os.Getpid())
	defer removePID()

	return d.Run(ctx)
}

func launchBackground() error {
	// Check for existing running instance
	if pid, alive := readPID(); alive {
		fmt.Printf("Daemon already running (PID %d)\n", pid)
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}

	child := exec.Command(exe, "serve", "--fg")
	child.Env = append(os.Environ(), "_SAME_TELEGRAM_BG=1")
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Redirect output to log file
	logPath := config.LogPath()
	os.MkdirAll(filepath.Dir(logPath), 0o755)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	child.Stdout = logFile
	child.Stderr = logFile

	if err := child.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start daemon: %w", err)
	}
	logFile.Close()

	// Wait for socket to appear (readiness check)
	socketPath := config.SocketPath()
	ready := false
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		conn, err := net.DialTimeout("unix", socketPath, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
	}

	if !ready {
		return fmt.Errorf("daemon failed to start within 5 seconds (check %s)", config.LogPath())
	}

	child.Process.Release()
	fmt.Printf("Daemon started (PID %d)\n", child.Process.Pid)
	fmt.Printf("  Socket: %s\n", socketPath)
	fmt.Printf("  Log:    %s\n", config.LogPath())
	return nil
}

func runServeStop(cmd *cobra.Command, args []string) error {
	pid, alive := readPID()
	if !alive {
		fmt.Println("Daemon is not running")
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM: %w", err)
	}

	// Wait for process to exit
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			break
		}
	}

	removePID()
	fmt.Printf("Daemon stopped (was PID %d)\n", pid)
	return nil
}

func runServeStatus(cmd *cobra.Command, args []string) error {
	pid, alive := readPID()
	if alive {
		fmt.Printf("Daemon is running (PID %d)\n", pid)
		fmt.Printf("  Socket: %s\n", config.SocketPath())
		fmt.Printf("  Log:    %s\n", config.LogPath())
	} else if pid > 0 {
		fmt.Printf("Daemon is not running (stale PID %d)\n", pid)
	} else {
		fmt.Println("Daemon is not running")
	}
	return nil
}

func writePID(pid int) {
	pidPath := config.PidPath()
	os.MkdirAll(filepath.Dir(pidPath), 0o755)
	tmpPath := pidPath + ".tmp"
	os.WriteFile(tmpPath, []byte(fmt.Sprintf("%d\n", pid)), 0o600)
	os.Rename(tmpPath, pidPath)
}

func removePID() {
	os.Remove(config.PidPath())
}

func readPID() (int, bool) {
	data, err := os.ReadFile(config.PidPath())
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return pid, false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return pid, false
	}
	return pid, true
}
