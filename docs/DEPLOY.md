# Deploying same-telegram to Fly.io

## Prerequisites

- [flyctl](https://fly.io/docs/flyctl/install/) installed and authenticated (`fly auth login`)
- A Telegram bot token from [@BotFather](https://t.me/BotFather)
- Your SAME encryption key (from `~/.same/encryption.key`)

## Quick Start

### 1. Launch the app

```bash
fly launch --no-deploy
```

Accept the defaults or customize. The `fly.toml` is already configured for DFW region, shared-cpu-1x, 256MB RAM.

### 2. Create persistent storage

```bash
fly volumes create same_data --size 1 --region dfw
```

This creates a 1GB volume mounted at `/data` inside the container. It holds the config, SQLite database, encryption key, and audit log.

### 3. Set secrets

```bash
fly secrets set \
  TELEGRAM_TOKEN="your-bot-token" \
  ENCRYPTION_KEY="your-encryption-key"
```

These are injected as environment variables at runtime. Never put them in `fly.toml`.

### 4. Deploy

```bash
fly deploy
```

### 5. Verify

```bash
fly logs        # watch bot startup
fly status      # check machine is running
```

## Configuration

The bot reads `~/.same/telegram.toml` at startup. In the Fly environment, `SAME_HOME` is set to `/data`, so the config path becomes `/data/telegram.toml`.

To seed the config on first deploy, you can use `fly ssh console` to write it manually, or let the bot create defaults and then configure via Telegram commands.

## Notes

### No HTTP service

same-telegram is a long-poll bot, not a web server. There are no HTTP services or health check endpoints configured. Fly keeps the machine running because `auto_stop_machines` is disabled.

### same CLI dependency

Full vault search operations require the `same` CLI to be available in PATH. The current Dockerfile does not include it. The bot can still run in API mode (onboarding, provider API key management) without the same CLI. To add vault search support, extend the Dockerfile to copy the `same` binary into the runtime image.

### Free tier

A single shared-cpu-1x machine with 256MB RAM and a 1GB volume fits within Fly.io's free allowance.

### Scaling

This is a single-machine deployment. The bot uses SQLite and unix sockets, so horizontal scaling is not applicable. For higher availability, rely on Fly's automatic machine restart on failure.
