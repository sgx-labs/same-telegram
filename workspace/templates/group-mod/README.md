# Group Moderator Bot

A Telegram bot that monitors group chats using AI to detect spam, toxicity,
and off-topic messages. Uses SAME to track repeat offenders with an
escalating response system: warn, mute, and audit trail.

## How It Works

1. Every group message is analyzed by AI for spam, toxicity, and relevance
2. Flagged messages get a warning; high-severity messages are deleted
3. Warnings are tracked in SAME per user
4. After reaching the warning threshold, the user is temporarily muted
5. All actions are logged in SAME for admin review

The bot errs on the side of permissiveness -- normal conversation, questions,
and friendly banter pass through without issue.

## Setup

### 1. Create your bot with BotFather

Message [@BotFather](https://t.me/BotFather) and run `/newbot`.
Copy the bot token.

### 2. Add the bot to your group

Add the bot to your Telegram group and make it an **admin** with these
permissions:
- Delete messages
- Restrict members (for muting)

### 3. Get an OpenRouter API key

Sign up at [openrouter.ai](https://openrouter.ai) and create an API key.

### 4. Deploy to Fly.io

```bash
fly launch --copy-config --no-deploy
fly volumes create bot_data --region iad --size 1

fly secrets set TELEGRAM_BOT_TOKEN="your-bot-token"
fly secrets set OPENROUTER_API_KEY="your-openrouter-key"

# Set your group's topic for off-topic detection
fly secrets set GROUP_TOPIC="Python programming and development"

fly deploy
```

## Commands (Admin Only)

| Command | Description |
|---------|-------------|
| `/modstats` | Show moderation statistics |
| `/history <user_id>` | Show a user's warning history |
| `/start` | Show bot info |

## Configuration

| Env Var | Description | Default |
|---------|-------------|---------|
| `TELEGRAM_BOT_TOKEN` | Bot token from BotFather | required |
| `OPENROUTER_API_KEY` | OpenRouter API key | required |
| `VAULT_PATH` | SAME vault storage path | `/data/vault` |
| `GROUP_TOPIC` | Group topic (for off-topic detection) | `general discussion` |
| `WARN_THRESHOLD` | Warnings before auto-mute | `3` |
| `MUTE_DURATION` | Mute duration in minutes | `60` |

## Moderation Logic

| Severity | Action |
|----------|--------|
| Low | Warning message with count |
| Medium | Warning message with count |
| High | Message deleted + warning |
| Threshold reached | User muted for `MUTE_DURATION` minutes |

Admin messages are never moderated. The bot skips messages from group
administrators and creators.
