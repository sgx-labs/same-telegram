#!/bin/bash
set -e

echo "=== SAME Dev Team - Post-Create Setup ==="

# Install Claude Code CLI (Node available via devcontainer feature)
echo "Installing Claude Code CLI..."
npm install -g @anthropic-ai/claude-code 2>/dev/null || echo "Claude Code install skipped"

# Install ollama for local AI
echo "Installing Ollama..."
curl -fsSL https://ollama.com/install.sh | sh 2>/dev/null || echo "Ollama install skipped"

# Ensure .same directories exist with correct permissions
mkdir -p ~/.same
chmod 700 ~/.same

# Install Go deps for all projects
for dir in /workspace/code/statelessagent /workspace/code/same-telegram; do
  if [ -f "$dir/go.mod" ]; then
    echo "Installing Go deps for $(basename $dir)..."
    cd "$dir" && go mod download
  fi
done

# Build and install SAME CLI
if [ -f "/workspace/code/statelessagent/Makefile" ]; then
  echo "Building SAME CLI..."
  cd /workspace/code/statelessagent && make build 2>/dev/null && sudo cp build/same /usr/local/bin/same && echo "SAME $(same version) installed"
  # Install guard hooks on all repos
  for repo in /workspace/code/statelessagent /workspace/code/same-telegram /workspace/code/SeedVaults /workspace/code/statelessagent.com; do
    if [ -d "$repo/.git" ]; then
      cd "$repo" && same guard install --force 2>/dev/null && same guard push-install --force 2>/dev/null
    fi
  done
  echo "SAME Guard installed on all repos"
fi

# Set up git identity for dev
git config --global user.name "sgx-labs"
git config --global user.email "dev@sgx-labs.dev"
git config --global init.defaultBranch main

# Build same-telegram
if [ -f "/workspace/code/same-telegram/Makefile" ]; then
  echo "Building same-telegram..."
  cd /workspace/code/same-telegram && make build 2>/dev/null && echo "same-telegram built"
fi

# Start Telegram bot in background (if config exists)
if [ -f "$HOME/.same/telegram.toml" ]; then
  echo "Starting Telegram bot..."
  /workspace/code/same-telegram/scripts/start-bot.sh
fi

# Pull a small ollama model for local AI testing (background, non-blocking)
if command -v ollama &>/dev/null; then
  echo "Starting Ollama and pulling qwen2.5-coder:3b in background..."
  nohup sh -c 'ollama serve &>/dev/null & sleep 3 && ollama pull qwen2.5-coder:3b' &>/dev/null &
fi

echo ""
echo "==========================================="
echo "  SAME Dev Team - Ready"
echo "==========================================="
echo ""
echo "  Repos:"
echo "    /workspace/code/statelessagent    (core CLI)"
echo "    /workspace/code/same-telegram     (Telegram plugin)"
echo "    /workspace/code/statelessagent.com (website)"
echo "    /workspace/same-company           (company HQ)"
echo ""
echo "  Quick start:"
echo "    cd /workspace/code/same-telegram && make build && ./same-telegram serve --fg"
echo ""
echo "  To open multi-root workspace:"
echo "    File > Open Workspace from File > /workspace/code/same-telegram/.devcontainer/same.code-workspace"
echo ""
