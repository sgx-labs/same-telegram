#!/bin/bash
# workspace/seeds/seed-same-demo.sh — Seeds the vault with SAME demo content.
#
# Creates a guided walkthrough that shows how persistent memory works.
# Called by seed-vault.sh; expects VAULT_PATH to be set.

set -euo pipefail

VAULT_PATH="${VAULT_PATH:-/data/vault}"

mkdir -p "$VAULT_PATH/notes"

# --- notes/welcome.md ---
if [ ! -f "$VAULT_PATH/notes/welcome.md" ]; then
cat > "$VAULT_PATH/notes/welcome.md" << 'EOF'
# Welcome to SAME

SAME (Stateless Agent Memory Engine) gives your AI agent persistent memory
that survives across sessions. Every conversation builds on the last.

## What is this vault?

This vault is your personal knowledge store. It lives on a persistent volume
attached to your workspace container. When your session ends — whether you
close the browser, disconnect, or the container restarts — your vault stays
intact. The next time you start a conversation, Claude picks up right where
you left off.

## How it works

1. **You talk to Claude** in the terminal.
2. **Claude reads and writes to this vault** using MCP tools (memory-read,
   memory-write, memory-search).
3. **The vault persists** between sessions — nothing is lost.

Think of it like giving Claude a notebook it can refer back to every time
you meet.

## Try it now

Ask Claude something like:

> "Remember that my favorite programming language is Rust."

Then close this session, start a new one, and ask:

> "What's my favorite programming language?"

Claude will know. That's persistent memory.
EOF
echo "  Created notes/welcome.md"
fi

# --- notes/getting-started.md ---
if [ ! -f "$VAULT_PATH/notes/getting-started.md" ]; then
cat > "$VAULT_PATH/notes/getting-started.md" << 'EOF'
# Getting Started

## Basic Commands

You interact with your vault through Claude. Just talk naturally — Claude
has MCP tools that let it read, write, and search your vault automatically.

### Storing information

Tell Claude anything you want remembered:

- "Remember that I'm working on a web scraper in Python."
- "Save this API endpoint: https://api.example.com/v2"
- "Note that the deadline for the project is March 15."

### Retrieving information

Ask Claude about things you've stored:

- "What projects am I working on?"
- "What was that API endpoint I saved?"
- "Summarize everything you know about me."

### Exploring the vault

You can also browse the vault directly in the terminal:

```bash
ls /data/vault/          # see top-level structure
find /data/vault -name "*.md" | head -20  # list markdown files
cat /data/vault/notes/welcome.md          # read a specific note
```

## Tips

- **Be specific** when asking Claude to remember things — it helps retrieval.
- **Ask Claude to organize** — it can create folders, move notes, and restructure.
- **The vault is just files** — markdown files on disk. No magic, no lock-in.
EOF
echo "  Created notes/getting-started.md"
fi

# --- notes/how-memory-works.md ---
if [ ! -f "$VAULT_PATH/notes/how-memory-works.md" ]; then
cat > "$VAULT_PATH/notes/how-memory-works.md" << 'EOF'
# How Memory Works

## The Problem

Large language models are stateless. Each conversation starts from scratch.
Claude doesn't inherently remember what you talked about yesterday.

## The Solution

SAME bridges this gap with a simple architecture:

```
You <-> Claude <-> MCP <-> SAME Vault (files on disk)
```

1. **MCP (Model Context Protocol)** connects Claude to external tools.
2. **SAME** exposes your vault as an MCP server with read/write/search tools.
3. **The vault** is a directory of markdown files on a persistent volume.

When Claude needs to remember something, it writes to the vault. When it
needs to recall something, it reads or searches the vault. The vault lives
on a volume that persists independently of the container.

## What persists

- Everything in `/data/vault/` — your notes, context, saved information.
- The vault structure — directories, files, organization.

## What doesn't persist

- Shell history and environment variables.
- Installed packages (apt, pip, npm) — the container image is read-only.
- Files outside `/data/` — anything in `/tmp`, `/home`, etc.

## Privacy

Your vault is yours. It lives on an isolated volume attached only to your
workspace container. No other users can access it.
EOF
echo "  Created notes/how-memory-works.md"
fi

# --- CLAUDE.md ---
if [ ! -f "$VAULT_PATH/CLAUDE.md" ]; then
cat > "$VAULT_PATH/CLAUDE.md" << 'EOF'
# SAME Demo Vault

This is a demo vault for SAME (Stateless Agent Memory Engine). The user is
exploring how persistent memory works.

## Your role

You are a helpful assistant demonstrating SAME's persistent memory capabilities.
Show the user how memory works by actively using your MCP tools to store and
retrieve information.

## Guidelines

- When the user tells you something worth remembering, store it in the vault.
- When the user asks about past interactions, search the vault first.
- Explain what you're doing — "I'm saving this to your vault" or "Let me
  check your vault for that."
- Encourage the user to test persistence by closing and reopening sessions.

## Vault structure

- `notes/welcome.md` — Introduction to SAME
- `notes/getting-started.md` — Basic usage guide
- `notes/how-memory-works.md` — Technical explanation

Feel free to create additional files and directories as the conversation
evolves.
EOF
echo "  Created CLAUDE.md"
fi
