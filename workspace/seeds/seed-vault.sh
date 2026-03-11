#!/bin/bash
# workspace/seeds/seed-vault.sh — Populates a vault with starter content
# based on a user-selected template.
#
# Usage: seed-vault.sh <seed-type> [topic]
#   seed-type: "same-demo", "research", "project", "empty"
#   topic:     optional free-text description (used by research/project)
#
# Expects VAULT_PATH to be set (defaults to /data/vault).
# Idempotent — safe to run multiple times.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VAULT_PATH="${VAULT_PATH:-/data/vault}"

# --- Argument parsing ---
SEED_TYPE="${1:-}"
shift || true
TOPIC="${*:-}"

if [ -z "$SEED_TYPE" ]; then
    echo "Usage: seed-vault.sh <seed-type> [topic]"
    echo "  seed-type: same-demo, research, project, bot-dev, empty"
    echo "  topic:     optional free-text description"
    exit 1
fi

# --- Slugify helper ---
# Converts free text to a filename-safe slug: lowercase, spaces→hyphens,
# strip everything except alphanumerics and hyphens, collapse runs.
slugify() {
    echo "$1" | tr '[:upper:]' '[:lower:]' \
              | sed 's/[^a-z0-9 -]//g' \
              | sed 's/  */ /g' \
              | sed 's/ /-/g' \
              | sed 's/--*/-/g' \
              | sed 's/^-//;s/-$//'
}

export VAULT_PATH
export TOPIC
export -f slugify

# --- Dispatch ---
case "$SEED_TYPE" in
    same-demo)
        echo "Seeding vault with SAME demo content..."
        "$SCRIPT_DIR/seed-same-demo.sh"
        ;;
    research)
        echo "Seeding vault with research template..."
        "$SCRIPT_DIR/seed-research.sh"
        ;;
    project)
        echo "Seeding vault with project template..."
        "$SCRIPT_DIR/seed-project.sh"
        ;;
    bot-dev)
        echo "Seeding vault with bot developer template..."
        "$SCRIPT_DIR/seed-bot-dev.sh"
        ;;
    empty)
        echo "Seeding vault with minimal setup..."
        "$SCRIPT_DIR/seed-empty.sh"
        ;;
    *)
        echo "Error: unknown seed type '$SEED_TYPE'"
        echo "Valid types: same-demo, research, project, bot-dev, empty"
        exit 1
        ;;
esac

echo "Vault seeded at $VAULT_PATH (type: $SEED_TYPE)."
