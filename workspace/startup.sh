#!/bin/bash
# workspace/startup.sh — Runs inside the user's container on first boot.
#
# Sets up the SAME vault, wires MCP config for Claude Code, and starts
# the workspace server. Designed to be idempotent — safe to run on every
# container start, not just the first time.

set -euo pipefail

VAULT_PATH="${VAULT_PATH:-/data/vault}"
SAME_HOME="${SAME_HOME:-/data}"

# --- Claude Code config persistence ---
# Claude Code stores auth credentials, settings, and MCP config in ~/.claude/.
# We symlink it to the persistent volume so logins (claude auth login) and
# config survive container restarts. Without this, users have to re-auth
# every time their machine stops and starts.
CLAUDE_DIR="$HOME/.claude"
CLAUDE_DATA="/data/.claude"
if [ ! -L "$CLAUDE_DIR" ]; then
    mkdir -p "$CLAUDE_DATA"
    # Preserve any config from the Docker image (e.g., Dockerfile-created dir).
    if [ -d "$CLAUDE_DIR" ]; then
        cp -a "$CLAUDE_DIR/." "$CLAUDE_DATA/" 2>/dev/null || true
        rm -rf "$CLAUDE_DIR"
    fi
    ln -s "$CLAUDE_DATA" "$CLAUDE_DIR"
    echo "Claude Code config linked to persistent volume."
fi

MCP_CONFIG="$CLAUDE_DIR/mcp.json"

# --- Vault ---
# Initialize the vault if it doesn't exist yet.
if [ ! -d "$VAULT_PATH" ]; then
    echo "Initializing your vault at $VAULT_PATH..."
    if command -v same >/dev/null 2>&1; then
        same init --path "$VAULT_PATH" 2>/dev/null || mkdir -p "$VAULT_PATH"
    else
        mkdir -p "$VAULT_PATH"
    fi
    echo "Vault ready."
else
    echo "Vault found at $VAULT_PATH."
fi

# --- MCP config ---
# Wire Claude Code -> SAME vault via MCP. This is what gives Claude
# persistent memory -- every session builds on the last.
mkdir -p "$(dirname "$MCP_CONFIG")"
if command -v same >/dev/null 2>&1; then
    cat > "$MCP_CONFIG" <<MCPEOF
{
  "mcpServers": {
    "memory": {
      "command": "same",
      "args": ["mcp"],
      "env": {
        "VAULT_PATH": "$VAULT_PATH"
      }
    }
  }
}
MCPEOF
    echo "MCP configured: Claude Code -> SAME vault."
else
    echo '{}' > "$MCP_CONFIG"
    echo "MCP config: same binary not found, skipping vault MCP wiring."
fi

# --- API keys ---
# Source persisted environment variables (API keys set during onboarding).
# Written by the bot's InjectAPIKey via Fly exec. Lives on the persistent
# volume so it survives container restarts.
if [ -f /data/.env ]; then
    set -a
    . /data/.env
    set +a
    echo "Loaded API keys from /data/.env."
fi

# --- tmux ---
# Ensure tmux server is running. The workspace-server will create the
# actual session on first WebSocket connect.
tmux start-server 2>/dev/null || true

# --- Workspace server ---
echo "Starting workspace server..."
exec workspace-server "$@"
