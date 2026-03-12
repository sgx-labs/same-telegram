# Customer Support Bot

A Telegram bot that answers questions from your knowledge base using SAME
for semantic search and AI for response synthesis.

## How It Works

1. On startup, the bot ingests all `.md` and `.txt` files from `knowledge/`
2. Content is chunked by paragraph and stored in SAME with semantic embeddings
3. When a user asks a question, SAME finds the most relevant knowledge chunks
4. AI synthesizes a natural-language answer from those chunks
5. If confidence is low (too few matching sources), the bot offers to escalate

## Setup

### 1. Add your knowledge base

Replace or add files in the `knowledge/` directory. Use Markdown or plain text.
Write clear, factual content — each paragraph becomes a searchable chunk.

### 2. Create your bot with BotFather

Open Telegram, message [@BotFather](https://t.me/BotFather), and run `/newbot`.
Copy the bot token.

### 3. Get an OpenRouter API key

Sign up at [openrouter.ai](https://openrouter.ai) and create an API key.

### 4. Deploy to Fly.io

```bash
fly launch --copy-config --no-deploy
fly volumes create bot_data --region iad --size 1

fly secrets set TELEGRAM_BOT_TOKEN="your-bot-token"
fly secrets set OPENROUTER_API_KEY="your-openrouter-key"

# Optional: set a chat ID to receive escalation notifications
fly secrets set ESCALATION_CHAT_ID="your-chat-id"

fly deploy
```

## Commands

| Command | Description |
|---------|-------------|
| `/start` | Welcome message |
| `/escalate` | Request a human support agent |
| `/help` | Show available commands |

## Configuration

| Env Var | Description | Default |
|---------|-------------|---------|
| `TELEGRAM_BOT_TOKEN` | Bot token from BotFather | required |
| `OPENROUTER_API_KEY` | OpenRouter API key | required |
| `VAULT_PATH` | SAME vault storage path | `/data/vault` |
| `ESCALATION_CHAT_ID` | Chat ID for human escalations | none |
| `CONFIDENCE_THRESHOLD` | Min sources needed to skip escalation hint | `2` |

## Tips

- Keep knowledge files focused — one topic per file works best
- Use clear headings and short paragraphs for better chunk quality
- Re-deploy after updating knowledge files (they're re-ingested on startup)
- Monitor escalation frequency to identify knowledge gaps
