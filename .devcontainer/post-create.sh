#!/bin/bash
set -e

echo "=== SAME Telegram Bot - Post-Create Setup ==="

# Install Claude Code CLI (Node available via devcontainer feature)
echo "Installing Claude Code CLI..."
npm install -g @anthropic-ai/claude-code 2>/dev/null || echo "Claude Code install skipped"

# Install ollama for local AI
echo "Installing Ollama..."
curl -fsSL https://ollama.com/install.sh | sh 2>/dev/null || echo "Ollama install skipped"

# Ensure .same directories exist with correct permissions
mkdir -p ~/.same
chmod 700 ~/.same

# Install Go deps
if [ -f "go.mod" ]; then
  echo "Installing Go deps..."
  go mod download
fi

# Set up git identity for dev
git config --global user.name "sgx-labs"
git config --global user.email "dev@sgx-labs.dev"
git config --global init.defaultBranch main

# Build same-telegram
if [ -f "Makefile" ]; then
  echo "Building same-telegram..."
  make build 2>/dev/null && echo "same-telegram built"
fi

# Start Telegram bot in background (if config exists)
if [ -f "$HOME/.same/telegram.toml" ]; then
  echo "Starting Telegram bot..."
  scripts/start-bot.sh
fi

# Pull a small ollama model for local AI testing (background, non-blocking)
if command -v ollama &>/dev/null; then
  echo "Starting Ollama and pulling qwen2.5-coder:3b in background..."
  nohup sh -c 'ollama serve &>/dev/null & sleep 3 && ollama pull qwen2.5-coder:3b' &>/dev/null &
fi

echo ""
echo "==========================================="
echo "  SAME Telegram Bot - Ready"
echo "==========================================="
echo ""
echo "  Quick start:"
echo "    make build && ./same-telegram serve --fg"
echo ""
