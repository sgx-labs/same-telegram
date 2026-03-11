#!/bin/bash
# workspace/seeds/seed-empty.sh — Seeds the vault with a minimal CLAUDE.md.
#
# Called by seed-vault.sh; expects VAULT_PATH to be set.

set -euo pipefail

VAULT_PATH="${VAULT_PATH:-/data/vault}"

mkdir -p "$VAULT_PATH"

# --- CLAUDE.md ---
if [ ! -f "$VAULT_PATH/CLAUDE.md" ]; then
cat > "$VAULT_PATH/CLAUDE.md" << 'EOF'
# Vault

This is an empty vault. You have persistent memory — anything you write here
will survive across sessions.

## Your role

You are a general-purpose assistant with persistent memory. Store useful
context in this vault so you can pick up where you left off in future sessions.

## Getting started

This vault has no predefined structure. Organize it however makes sense for
the conversation. Some suggestions:

- Create a `notes/` directory for general information.
- Create a `context/` directory for session summaries.
- Use markdown files for everything — they're easy to read and search.

When the user tells you something worth remembering, save it. When they ask
about something from a previous session, search the vault.
EOF
echo "  Created CLAUDE.md"
fi
