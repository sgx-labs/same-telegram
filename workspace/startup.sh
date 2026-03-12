#!/bin/bash
# workspace/startup.sh — Runs inside the user's container on first boot.
#
# Sets up the SAME vault, wires MCP config for Claude Code, and starts
# the workspace server. Designed to be idempotent — safe to run on every
# container start, not just the first time.

set -uo pipefail
# Note: we intentionally do NOT use 'set -e'. Individual persist_dir /
# persist_file calls should be non-fatal — a symlink race or permission
# glitch must not prevent the workspace server from starting.

VAULT_PATH="${VAULT_PATH:-/data/vault}"
SAME_HOME="${SAME_HOME:-/data}"

# --- Home directory persistence ---
# Key directories under /home/workspace are symlinked to /data/home/ so that
# user data (Claude config, shell history, project files) survives image
# updates. The persistent volume at /data is preserved across machine updates;
# the container filesystem is not.
DATA_HOME="/data/home"
mkdir -p "$DATA_HOME"

# persist_dir SRC_NAME
#   Ensures /data/home/<SRC_NAME> exists, migrates any existing data from
#   $HOME/<SRC_NAME>, and replaces it with a symlink to the persistent copy.
persist_dir() {
    local name="$1"
    local src="$HOME/$name"
    local dst="$DATA_HOME/$name"

    # Already symlinked — nothing to do.
    if [ -L "$src" ]; then
        return
    fi

    mkdir -p "$dst"

    # Migrate existing data from the image into the persistent location.
    if [ -d "$src" ]; then
        cp -a "$src/." "$dst/" 2>/dev/null || true
        rm -rf "$src"
    fi

    ln -s "$dst" "$src"
}

# persist_file SRC_NAME
#   Same as persist_dir but for individual files (e.g., .bash_history).
persist_file() {
    local name="$1"
    local src="$HOME/$name"
    local dst="$DATA_HOME/$name"

    if [ -L "$src" ]; then
        return
    fi

    # Migrate existing file content.
    if [ -f "$src" ] && [ ! -f "$dst" ]; then
        cp -a "$src" "$dst" 2>/dev/null || true
    fi

    # Create the file on the volume if it doesn't exist yet.
    touch "$dst"

    # Replace with symlink.
    rm -f "$src"
    ln -s "$dst" "$src"
}

persist_dir  ".claude"   || echo "WARNING: failed to persist .claude (non-fatal)"
persist_dir  "projects"  || echo "WARNING: failed to persist projects (non-fatal)"
persist_file ".bash_history" || echo "WARNING: failed to persist .bash_history (non-fatal)"

# Ensure correct ownership on the persistent home tree.
chown -R workspace:workspace "$DATA_HOME" 2>/dev/null || true

echo "Home directories linked to persistent volume."

# CLAUDE_DIR points to ~/.claude (now a symlink to /data/home/.claude).
CLAUDE_DIR="$HOME/.claude"
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

# --- Workspace server ---
echo "Starting workspace server..."
exec workspace-server "$@"
