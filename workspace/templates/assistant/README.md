# Personal AI Assistant Bot

A Telegram bot with AI-powered responses and persistent memory via SAME.

The bot remembers your conversations across sessions — preferences, facts,
and context carry over every time you chat.

## Setup

### 1. Create your bot with BotFather

Open Telegram, message [@BotFather](https://t.me/BotFather), and run:
- `/newbot` — follow the prompts to name your bot
- Copy the bot token it gives you

### 2. Get an OpenRouter API key

Sign up at [openrouter.ai](https://openrouter.ai) and create an API key.

### 3. Deploy to Fly.io

```bash
# First time: create the app and volume
fly launch --copy-config --no-deploy
fly volumes create bot_data --region iad --size 1

# Set your secrets
fly secrets set TELEGRAM_BOT_TOKEN="your-bot-token"
fly secrets set OPENROUTER_API_KEY="your-openrouter-key"

# Deploy
fly deploy
```

## Commands

| Command | Description |
|---------|-------------|
| `/start` | Welcome message |
| `/model <name>` | Switch AI model (e.g., `openai/gpt-4o`) |
| `/model` | Show current model |
| `/forget` | Clear all bot memories |
| `/help` | Show available commands |

## Configuration

| Env Var | Description | Default |
|---------|-------------|---------|
| `TELEGRAM_BOT_TOKEN` | Bot token from BotFather | required |
| `OPENROUTER_API_KEY` | OpenRouter API key | required |
| `VAULT_PATH` | SAME vault storage path | `/data/vault` |
| `SAME_API_URL` | SAME API endpoint (if remote) | local CLI |

## How Memory Works

Every conversation exchange is stored in SAME with semantic search.
When you send a message, the bot:

1. Searches SAME for relevant past memories
2. Includes those memories as context for the AI
3. Stores the new exchange for future reference

Use `/forget` to wipe all memories and start fresh.
