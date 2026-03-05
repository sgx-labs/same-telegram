#!/bin/bash
# stop-bot.sh — Gracefully stop same-telegram
set -euo pipefail

PID_FILE="$HOME/.same/telegram.pid"

if [ ! -f "$PID_FILE" ]; then
  echo "No PID file found — bot is not running"
  exit 0
fi

PID=$(cat "$PID_FILE")

if ! kill -0 "$PID" 2>/dev/null; then
  echo "Process $PID not running — cleaning up stale PID file"
  rm -f "$PID_FILE"
  exit 0
fi

echo "Sending SIGTERM to $PID..."
rm -f "$PID_FILE"  # Remove first so restart loop knows it's intentional
kill "$PID" 2>/dev/null || true

# Wait up to 10 seconds for graceful shutdown
for i in $(seq 1 10); do
  if ! kill -0 "$PID" 2>/dev/null; then
    echo "Bot stopped gracefully"
    exit 0
  fi
  sleep 1
done

# Still alive — force kill
echo "Bot did not stop in 10s — sending SIGKILL"
kill -9 "$PID" 2>/dev/null || true
echo "Bot killed"
