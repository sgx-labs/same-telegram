#!/bin/bash
# start-bot.sh — Start same-telegram with restart loop and PID tracking
set -euo pipefail

PROJ_DIR="${PROJ_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
PID_DIR="$HOME/.same"
PID_FILE="$PID_DIR/telegram.pid"
BINARY="$PROJ_DIR/same-telegram"
MAX_RETRIES=5
RETRY_WAIT=5

# Required env vars
export SAME_DATA_DIR="${SAME_DATA_DIR:-$HOME/.same/data}"
export VAULT_PATH="${VAULT_PATH:-$HOME/.same/vault}"

mkdir -p "$PID_DIR"

# Check if already running
if [ -f "$PID_FILE" ]; then
  OLD_PID=$(cat "$PID_FILE")
  if kill -0 "$OLD_PID" 2>/dev/null; then
    echo "Bot already running (PID $OLD_PID)"
    exit 0
  fi
  echo "Stale PID file found, cleaning up"
  rm -f "$PID_FILE"
fi

# Build if binary missing or older than source
needs_build() {
  [ ! -f "$BINARY" ] && return 0
  # Check if any .go file is newer than the binary
  if find "$PROJ_DIR" -name '*.go' -newer "$BINARY" -print -quit | grep -q .; then
    return 0
  fi
  return 1
}

if needs_build; then
  echo "Building same-telegram..."
  cd "$PROJ_DIR" && make build
fi

# Start with restart loop in background
(
  retries=0
  while [ "$retries" -lt "$MAX_RETRIES" ]; do
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Starting same-telegram (attempt $((retries + 1))/$MAX_RETRIES)..."
    cd "$PROJ_DIR"
    ./same-telegram serve --fg &
    BOT_PID=$!
    echo "$BOT_PID" > "$PID_FILE"

    # Wait for the bot process to exit
    wait "$BOT_PID" 2>/dev/null
    EXIT_CODE=$?

    # If PID file was removed, stop was intentional
    if [ ! -f "$PID_FILE" ]; then
      echo "PID file removed — clean shutdown"
      exit 0
    fi

    # Exit code 0 means clean exit, don't restart
    if [ "$EXIT_CODE" -eq 0 ]; then
      echo "Bot exited cleanly (code 0)"
      rm -f "$PID_FILE"
      exit 0
    fi

    retries=$((retries + 1))
    echo "Bot died (exit code $EXIT_CODE). Retry $retries/$MAX_RETRIES in ${RETRY_WAIT}s..."
    sleep "$RETRY_WAIT"
  done

  echo "Max retries ($MAX_RETRIES) exhausted. Giving up."
  rm -f "$PID_FILE"
) &

# Write the wrapper PID so stop script can kill the whole group
WRAPPER_PID=$!
echo "$WRAPPER_PID" > "$PID_FILE"
echo "Bot starting in background (wrapper PID $WRAPPER_PID)"
