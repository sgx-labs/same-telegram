# same-telegram

Telegram plugin for [SAME](https://github.com/sgx-labs/statelessagent) — manage your vault from your phone.

`same-telegram` turns Telegram into a remote management GUI for SAME. It runs as a Claude Code hook plugin (push notifications) and a Telegram bot daemon (pull commands).

## Quick Start

```bash
# Install
go install github.com/sgx-labs/same-telegram/cmd/same-telegram@latest

# Configure (interactive wizard)
same-telegram setup

# Start the daemon
same-telegram serve

# Message your bot on Telegram
/status
```

## How It Works

```
┌─────────────┐     stdin JSON      ┌──────────────┐    unix socket     ┌──────────────┐
│ Claude Code  │ ──────────────────→ │   hook cmd   │ ─────────────────→ │    daemon     │
│   (hooks)    │                     │ same-telegram │                    │  bot + socket │
└─────────────┘                     │    hook       │                    │    server     │
                                    └──────────────┘                    └──────┬───────┘
                                                                               │
                                                                    Telegram Bot API
                                                                               │
                                                                        ┌──────▼───────┐
                                                                        │   Your phone  │
                                                                        └──────────────┘
```

**Two modes in one binary:**

- **`same-telegram hook`** — Plugin mode. Receives Claude Code hook events on stdin, forwards them to the daemon via unix socket. Sub-millisecond, non-blocking.
- **`same-telegram serve`** — Daemon mode. Runs the Telegram bot (long-polling) and a unix socket server. Shells out to the `same` CLI for all vault operations.

## Setup

### 1. Create a Telegram Bot

1. Open Telegram and message [@BotFather](https://t.me/BotFather)
2. Send `/newbot` and follow the prompts
3. Copy the bot token (looks like `123456:ABC-DEF...`)

### 2. Find Your Telegram User ID

1. Message [@userinfobot](https://t.me/userinfobot) on Telegram
2. It replies with your numeric user ID

### 3. Run Setup

```bash
same-telegram setup
```

This creates `~/.same/telegram.toml` with your bot token and user ID whitelist.

### 4. Register as a SAME Plugin

Add to your vault's `.same/plugins.json`:

```json
{
  "plugins": [
    {
      "name": "telegram",
      "event": "Stop",
      "command": "same-telegram",
      "args": ["hook"],
      "timeout_ms": 5000,
      "enabled": true
    }
  ]
}
```

### 5. Start the Daemon

```bash
# Background (default)
same-telegram serve

# Foreground (for debugging)
same-telegram serve --fg

# Check status
same-telegram serve status

# Stop
same-telegram serve stop
```

## Telegram Commands

| Command | Description |
|---------|-------------|
| `/status` | Vault status overview |
| `/doctor` | Run SAME health check |
| `/search <query>` | Semantic search across your vault |
| `/ask <question>` | Ask SAME a question |
| `/vaults` | List and switch vaults |
| `/digest` | On-demand activity summary |
| `/config` | View current bot settings |
| `/help` | List all commands |

Any non-command text is treated as a search query.

## Notifications

When registered as a SAME plugin, you receive push notifications for:

- **Session ended** — Summary of work done
- **Decision logged** — With Approve / Add Note inline buttons
- **Agent handoff** — Context forwarded to next agent
- **Daily digest** — Configurable time, overnight vault activity

## Configuration

Config lives at `~/.same/telegram.toml`:

```toml
[bot]
token = "your-bot-token"
allowed_user_ids = [12345]  # Telegram user IDs

[notify]
session_end = true
decisions = true
handoffs = true

[digest]
enabled = true
time = "08:00"  # HH:MM local time
```

## Architecture

- **No SAME internals imported** — shells out to `same` CLI for all operations. Keeps the binary small (~6MB) and forward-compatible.
- **Unix socket IPC** — Hook commands communicate with the daemon via `~/.same/telegram.sock`. No port allocation, filesystem-level auth.
- **User ID whitelist** — Only configured Telegram user IDs can interact. Unknown users are silently dropped.
- **Atomic PID file** — Daemon writes PID to `~/.same/telegram.pid` via tmp+rename for crash safety.

## Development

```bash
make build     # Build binary
make test      # Run tests
make install   # Install to ~/go/bin/
make release   # Cross-compile for all platforms
```

Requires Go 1.25+.

## Security

- Bot token stored in `~/.same/telegram.toml` with `0600` permissions
- Unix socket created with `0600` permissions
- All Telegram interactions gated by user ID whitelist
- No vault data stored in the binary or config — all data fetched on-demand via `same` CLI

## License

Same license as [SAME](https://github.com/sgx-labs/statelessagent) — BSL 1.1 (converts to Apache 2.0 on 2030-02-02).
