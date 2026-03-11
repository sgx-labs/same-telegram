#!/bin/bash
# workspace/seeds/seed-bot-dev.sh — Seeds the vault with Telegram bot development resources.
#
# If TOPIC is set, uses it as the bot description.
# Called by seed-vault.sh; expects VAULT_PATH and optionally TOPIC to be set.

set -euo pipefail

VAULT_PATH="${VAULT_PATH:-/data/vault}"
TOPIC="${TOPIC:-}"

mkdir -p "$VAULT_PATH/notes"

# --- Determine bot description ---
if [ -n "$TOPIC" ]; then
    BOT_DISPLAY="$TOPIC"
else
    BOT_DISPLAY="New Telegram Bot"
fi

# --- CLAUDE.md ---
if [ ! -f "$VAULT_PATH/CLAUDE.md" ]; then
if [ -n "$TOPIC" ]; then
cat > "$VAULT_PATH/CLAUDE.md" << INNEREOF
# Bot Developer Workspace

You are a Telegram bot development expert. The user is building the following bot:

> $BOT_DISPLAY

## Your role

Help the user design, build, and ship this Telegram bot. You have persistent
memory — use it to track architecture decisions, progress, and context across
sessions.

## Guidelines

- Consult \`notes/\` for quick reference on the Bot API, Mini Apps, payments,
  performance, architecture patterns, and deployment.
- When making design decisions, document them so future sessions have context.
- Write code directly in the terminal. The user's workspace persists.
- Be opinionated: suggest proven patterns, flag common pitfalls early.
- When the user returns after a break, review the vault to catch up on context.

## Key patterns to follow

- Use callback data prefixes for routing (e.g., \`settings:\`, \`order:\`).
- Implement conversation state machines for multi-step flows.
- Always handle errors gracefully — show user-friendly messages, log details.
- Respect rate limits: 30 messages/second globally, 20 messages/minute per chat.
- Use webhook mode in production, long polling in development.

## Vault structure

\`\`\`
notes/
  telegram-bot-api.md — Bot API quick reference
  mini-apps.md        — Mini App / WebApp development
  monetization.md     — Telegram Stars and payments
  performance.md      — Rate limits, caching, optimization
  architecture.md     — Command routing, state machines, middleware
  deployment.md       — Hosting, webhooks, Docker, monitoring
\`\`\`

## Current project

**$BOT_DISPLAY**

Start by helping the user define the bot's command structure, identify which
Bot API features are needed, and scaffold the initial codebase.
INNEREOF
else
cat > "$VAULT_PATH/CLAUDE.md" << 'INNEREOF'
# Bot Developer Workspace

You are a Telegram bot development expert. This vault is set up for building
Telegram bots with persistent context tracking.

## Your role

Help the user design, build, and ship Telegram bots. You have persistent
memory — use it to track architecture decisions, progress, and context across
sessions.

## Guidelines

- Consult `notes/` for quick reference on the Bot API, Mini Apps, payments,
  performance, architecture patterns, and deployment.
- When making design decisions, document them so future sessions have context.
- Write code directly in the terminal. The user's workspace persists.
- Be opinionated: suggest proven patterns, flag common pitfalls early.
- When the user returns after a break, review the vault to catch up on context.

## Key patterns to follow

- Use callback data prefixes for routing (e.g., `settings:`, `order:`).
- Implement conversation state machines for multi-step flows.
- Always handle errors gracefully — show user-friendly messages, log details.
- Respect rate limits: 30 messages/second globally, 20 messages/minute per chat.
- Use webhook mode in production, long polling in development.

## Vault structure

```
notes/
  telegram-bot-api.md — Bot API quick reference
  mini-apps.md        — Mini App / WebApp development
  monetization.md     — Telegram Stars and payments
  performance.md      — Rate limits, caching, optimization
  architecture.md     — Command routing, state machines, middleware
  deployment.md       — Hosting, webhooks, Docker, monitoring
```

Ask the user what kind of bot they want to build, then help them define the
command structure, identify which Bot API features are needed, and scaffold
the initial codebase.
INNEREOF
fi
echo "  Created CLAUDE.md"
fi

# --- notes/telegram-bot-api.md ---
if [ ! -f "$VAULT_PATH/notes/telegram-bot-api.md" ]; then
cat > "$VAULT_PATH/notes/telegram-bot-api.md" << 'INNEREOF'
# Telegram Bot API Quick Reference

## Core methods

| Method | Purpose |
|--------|---------|
| `getMe` | Test bot token, get bot info |
| `sendMessage` | Send text (Markdown/HTML) |
| `sendPhoto/Document/Video/Audio` | Send media |
| `sendMediaGroup` | Send album (2-10 items) |
| `forwardMessage` | Forward existing message |
| `copyMessage` | Copy without forward header |
| `deleteMessage` | Delete a message |
| `editMessageText` | Edit sent message text |
| `editMessageReplyMarkup` | Update inline keyboard |
| `answerCallbackQuery` | Acknowledge button press |
| `sendChatAction` | Show "typing..." indicator |

## Update types

Updates arrive via webhook or `getUpdates`. Key fields:

```json
{
  "update_id": 123,
  "message": {},
  "edited_message": {},
  "callback_query": {},
  "inline_query": {},
  "chosen_inline_result": {},
  "chat_member": {},
  "my_chat_member": {},
  "pre_checkout_query": {},
  "poll": {},
  "poll_answer": {}
}
```

## Message object — key fields

```json
{
  "message_id": 456,
  "from": {"id": 789, "first_name": "User", "username": "user"},
  "chat": {"id": -100123, "type": "private|group|supergroup|channel"},
  "date": 1700000000,
  "text": "/start payload",
  "entities": [{"type": "bot_command", "offset": 0, "length": 6}],
  "reply_to_message": {},
  "photo": [{"file_id": "...", "width": 800, "height": 600}],
  "document": {"file_id": "...", "file_name": "report.pdf"}
}
```

## Inline keyboards

```json
{
  "reply_markup": {
    "inline_keyboard": [
      [
        {"text": "Button", "callback_data": "action:value"},
        {"text": "Link", "url": "https://example.com"}
      ],
      [
        {"text": "Open App", "web_app": {"url": "https://app.example.com"}}
      ]
    ]
  }
}
```

Callback data max: **64 bytes**. Use short prefixes.

## Inline mode

1. User types `@yourbot query` in any chat.
2. Bot receives `inline_query` with `query` string.
3. Bot calls `answerInlineQuery` with results array.

```json
{
  "inline_query_id": "abc",
  "results": [
    {
      "type": "article",
      "id": "1",
      "title": "Result Title",
      "input_message_content": {"message_text": "Sent text"},
      "reply_markup": {}
    }
  ],
  "cache_time": 300
}
```

## Commands setup

Register commands with `setMyCommands`:

```json
{
  "commands": [
    {"command": "start", "description": "Start the bot"},
    {"command": "help", "description": "Show help"},
    {"command": "settings", "description": "Bot settings"}
  ],
  "scope": {"type": "default"}
}
```

Scopes: `default`, `all_private_chats`, `all_group_chats`, `chat` (specific chat).

## File handling

- Download: `getFile` returns `file_path`, then `https://api.telegram.org/file/bot<token>/<file_path>`
- Upload limit: 50 MB (download), 10 MB (photos), 50 MB (other via multipart)
- Use `file_id` to resend without re-uploading

## Chat management

| Method | Purpose |
|--------|---------|
| `banChatMember` | Ban user from group |
| `restrictChatMember` | Set permissions |
| `promoteChatMember` | Give admin rights |
| `getChatMember` | Check user status |
| `getChatMemberCount` | Count members |
| `setChatTitle/Description/Photo` | Edit group info |
| `pinChatMessage` | Pin a message |
| `createChatInviteLink` | Generate invite link |
INNEREOF
echo "  Created notes/telegram-bot-api.md"
fi

# --- notes/mini-apps.md ---
if [ ! -f "$VAULT_PATH/notes/mini-apps.md" ]; then
cat > "$VAULT_PATH/notes/mini-apps.md" << 'INNEREOF'
# Telegram Mini Apps (WebApps)

## Opening a Mini App

1. **Inline keyboard button**: `{"text": "Open", "web_app": {"url": "..."}}`
2. **Menu button**: `setChatMenuButton` with `type: "web_app"`
3. **Inline mode**: `answerInlineQuery` with `button: {"text": "Open", "web_app": {"url": "..."}}`
4. **Direct link**: `https://t.me/botname/appname`

## SDK initialization

```html
<script src="https://telegram.org/js/telegram-web-app.js"></script>
<script>
  const tg = window.Telegram.WebApp;
  tg.ready(); // Tell Telegram the app is ready
  tg.expand(); // Expand to full height
</script>
```

## User data and validation

```javascript
// initData string (URL-encoded) — validate on your server
const initData = tg.initData;
const user = tg.initDataUnsafe.user;
// { id, first_name, last_name, username, language_code, is_premium }

// Server-side validation: HMAC-SHA256 with bot token
// 1. Parse initData as URLSearchParams
// 2. Sort params alphabetically (exclude "hash")
// 3. Create data_check_string = "key=value\n..."
// 4. secret_key = HMAC-SHA256("WebAppData", bot_token)
// 5. hash = HMAC-SHA256(secret_key, data_check_string)
// 6. Compare hash with received hash param
```

## Theme parameters

```javascript
tg.themeParams = {
  bg_color, text_color, hint_color, link_color, button_color,
  button_text_color, secondary_bg_color, header_bg_color,
  accent_text_color, section_bg_color, section_header_text_color,
  subtitle_text_color, destructive_text_color
};
// Use CSS variables: var(--tg-theme-bg-color), etc.
```

## Main Button

```javascript
tg.MainButton.text = "Submit";
tg.MainButton.color = "#2481cc";
tg.MainButton.show();
tg.MainButton.onClick(() => {
  tg.MainButton.showProgress();
  // do work...
  tg.MainButton.hideProgress();
});
tg.MainButton.hide();
```

## Back Button

```javascript
tg.BackButton.show();
tg.BackButton.onClick(() => {
  // navigate back in your app
});
tg.BackButton.hide();
```

## Haptic feedback

```javascript
tg.HapticFeedback.impactOccurred("light"); // light, medium, heavy, rigid, soft
tg.HapticFeedback.notificationOccurred("success"); // success, error, warning
tg.HapticFeedback.selectionChanged();
```

## Cloud storage (per-user, per-bot)

```javascript
tg.CloudStorage.setItem("key", "value", (err, ok) => {});
tg.CloudStorage.getItem("key", (err, value) => {});
tg.CloudStorage.getItems(["k1", "k2"], (err, values) => {});
tg.CloudStorage.removeItem("key", (err, ok) => {});
tg.CloudStorage.getKeys((err, keys) => {});
// Limits: 1024 keys, key max 128 bytes, value max 4096 bytes
```

## Biometric authentication

```javascript
tg.BiometricManager.init(() => {
  if (tg.BiometricManager.isInited && tg.BiometricManager.isBiometricAvailable) {
    tg.BiometricManager.requestAccess({reason: "Verify identity"}, (granted) => {
      if (granted) {
        tg.BiometricManager.authenticate({reason: "Confirm action"}, (ok, token) => {
          // token is the stored biometric token
        });
      }
    });
  }
});
```

## Viewport control

```javascript
tg.expand(); // Expand to maximum height
tg.isExpanded; // Check if expanded
tg.viewportHeight; // Current viewport height
tg.viewportStableHeight; // Height excluding keyboard
tg.onEvent("viewportChanged", ({isStateStable}) => {});
```

## Sending data back to bot

```javascript
// Simple data (closes the Mini App)
tg.sendData(JSON.stringify({action: "submit", value: 42}));
// Bot receives it as message.web_app_data.data

// Or use your own API endpoint and send results via bot API
```

## Closing

```javascript
tg.close(); // Close the Mini App
tg.enableClosingConfirmation(); // Ask before closing
tg.disableClosingConfirmation();
```
INNEREOF
echo "  Created notes/mini-apps.md"
fi

# --- notes/monetization.md ---
if [ ! -f "$VAULT_PATH/notes/monetization.md" ]; then
cat > "$VAULT_PATH/notes/monetization.md" << 'INNEREOF'
# Telegram Stars & Monetization

## Telegram Stars overview

- Digital currency for in-app purchases within bots and Mini Apps.
- Users buy Stars with real money through Telegram.
- Bot developers receive ~70% of Star value (Apple/Google take ~30% on mobile).
- Stars can be withdrawn as TON (Toncoin) via Fragment.

## Sending an invoice

```json
// sendInvoice
{
  "chat_id": 123,
  "title": "Premium Access",
  "description": "Unlock all features for 30 days",
  "payload": "premium_30d_user123",
  "currency": "XTR",
  "prices": [{"label": "Premium (30 days)", "amount": 100}],
  "provider_token": ""
}
```

- `currency` must be `"XTR"` for Stars.
- `provider_token` must be empty string for Stars.
- `payload` is your internal reference (up to 128 bytes), not shown to user.
- `amount` is in Stars (1 = 1 Star, no decimals).

## Payment flow

1. Bot sends invoice via `sendInvoice` or inline keyboard with `pay: true`.
2. User sees payment dialog, confirms.
3. Bot receives `pre_checkout_query`.
4. Bot calls `answerPreCheckoutQuery` with `ok: true` (or `error_message`).
5. Bot receives `message` with `successful_payment` field.

```json
// successful_payment in message
{
  "currency": "XTR",
  "total_amount": 100,
  "invoice_payload": "premium_30d_user123",
  "telegram_payment_charge_id": "charge_abc123"
}
```

## Handling pre_checkout_query

Always respond within **10 seconds** or the payment fails.

```json
// answerPreCheckoutQuery — accept
{"pre_checkout_query_id": "abc", "ok": true}

// answerPreCheckoutQuery — reject
{"pre_checkout_query_id": "abc", "ok": false, "error_message": "Item out of stock"}
```

## Refunds

```json
// refundStarPayment
{
  "user_id": 123,
  "telegram_payment_charge_id": "charge_abc123"
}
```

- Refund within 21 days of purchase.
- Stars are returned to the user.
- You cannot refund partial amounts.

## Subscription patterns

Stars do not have built-in subscriptions. Implement manually:

1. Store `successful_payment.telegram_payment_charge_id` and expiry date.
2. Send renewal invoices before expiry (e.g., 3 days before).
3. Track active/expired status in your database.
4. Check on each request: `if subscription.expires_at < now: prompt_renewal()`.

## Inline invoice button

```json
{
  "inline_keyboard": [[
    {"text": "Pay 50 Stars", "pay": true}
  ]]
}
```

The `pay: true` button must be the first button in the first row.

## Stars balance check

Use `getStarTransactions` to list recent transactions:

```json
// getStarTransactions
{"offset": 0, "limit": 100}

// Response
{
  "transactions": [
    {
      "id": "tx_123",
      "amount": 100,
      "date": 1700000000,
      "source": {"type": "user", "user": {"id": 789}},
      "receiver": {"type": "bot", "user": {"id": 456}}
    }
  ]
}
```
INNEREOF
echo "  Created notes/monetization.md"
fi

# --- notes/performance.md ---
if [ ! -f "$VAULT_PATH/notes/performance.md" ]; then
cat > "$VAULT_PATH/notes/performance.md" << 'INNEREOF'
# Performance & Rate Limits

## Rate limits

| Scope | Limit |
|-------|-------|
| Global (all chats) | 30 messages/second |
| Per chat (private) | 1 message/second (soft, bursts OK) |
| Per group chat | 20 messages/minute |
| `sendMessage` to same chat | ~1/sec sustained |
| Bulk notifications | 30 users/second |
| `getUpdates` | No meaningful limit |
| Inline query answers | No strict limit, but cache aggressively |

When exceeded: API returns 429 with `retry_after` seconds.

## Webhook vs long polling

### Webhook (production)
```
setWebhook:
  url: "https://bot.example.com/webhook/<secret>"
  max_connections: 40
  allowed_updates: ["message", "callback_query"]
  secret_token: "random_string_for_header_check"
```

**Pros**: Real-time, no polling overhead, scales naturally.
**Cons**: Needs HTTPS, public URL, SSL cert.

- Respond directly in webhook response body (saves one API call).
- Set `allowed_updates` to receive only what you handle.
- Use `secret_token` — Telegram sends it as `X-Telegram-Bot-Api-Secret-Token` header.

### Long polling (development)
```
getUpdates:
  offset: <last_update_id + 1>
  timeout: 30
  allowed_updates: ["message", "callback_query"]
```

**Pros**: No public URL needed, works behind NAT/firewall.
**Cons**: Slight delay, wastes connections.

- Always set `timeout` > 0 for long polling (avoid busy loops).
- Track `offset` to avoid reprocessing updates.

## Bulk messaging strategies

Sending to many users (e.g., broadcast):

1. **Queue-based**: Push user IDs to a queue, workers send at 30/sec.
2. **Batch with delay**: Send 30 messages, sleep 1 second, repeat.
3. **Store `chat_id` failures**: Remove users who blocked the bot (403 error).
4. **Use `copyMessage`** instead of `sendMessage` for forwarding content efficiently.

```
// Rate-limited sender pseudocode
for batch in chunks(users, 30):
    for user in batch:
        try: sendMessage(user, text)
        except 403: mark_blocked(user)
        except 429 as e: sleep(e.retry_after)
    sleep(1)
```

## Connection pooling

- Reuse HTTP connections to `api.telegram.org` (keep-alive).
- Most HTTP clients do this by default. Verify your library does.
- Set connection pool size: 10-50 depending on throughput.

## Caching strategies

| What to cache | TTL | Why |
|---------------|-----|-----|
| `getMe` result | Forever (until restart) | Never changes |
| `getChatMember` | 5-60 seconds | Permissions change rarely |
| `getChat` | 30 seconds | Chat info is mostly static |
| User settings | Until modified | Avoid redundant DB reads |
| Inline query results | Set `cache_time` in response | Telegram caches client-side |
| File IDs | Forever | File IDs don't expire |

## Response optimization

- Reply in webhook response body: saves one HTTP round-trip.
- Use `parse_mode: "HTML"` if building dynamic strings (easier escaping than Markdown).
- Batch edits: if updating a message repeatedly, throttle to max 1 edit/second.
- Use `sendChatAction` sparingly — it auto-expires after 5 seconds.

## Error handling

| Status code | Meaning | Action |
|-------------|---------|--------|
| 200 | Success | Process result |
| 400 | Bad request | Fix parameters, don't retry |
| 401 | Unauthorized | Check bot token |
| 403 | Forbidden | User blocked bot, or bot not in chat |
| 404 | Not found | Wrong method name |
| 409 | Conflict | Two instances using same token (getUpdates) |
| 429 | Rate limited | Wait `retry_after` seconds |
| 500+ | Server error | Retry with exponential backoff |
INNEREOF
echo "  Created notes/performance.md"
fi

# --- notes/architecture.md ---
if [ ! -f "$VAULT_PATH/notes/architecture.md" ]; then
cat > "$VAULT_PATH/notes/architecture.md" << 'INNEREOF'
# Bot Architecture Patterns

## Command routing

Route `/command` messages by parsing `message.entities` of type `bot_command`:

```python
# Simple router pattern
COMMANDS = {
    "/start": handle_start,
    "/help": handle_help,
    "/settings": handle_settings,
}

def route_message(update):
    text = update.message.text
    if text and text.startswith("/"):
        cmd = text.split()[0].split("@")[0]  # strip @botname
        handler = COMMANDS.get(cmd, handle_unknown)
        handler(update)
    else:
        handle_text(update)
```

## Callback routing with prefixes

Use structured prefixes in `callback_data` (max 64 bytes):

```
Format: "prefix:action:id" or "p:a:id" (keep short)

Examples:
  "settings:lang:en"     — settings menu, set language to English
  "order:cancel:abc123"  — cancel order abc123
  "page:3"               — pagination, go to page 3
```

```python
def route_callback(query):
    data = query.data
    prefix = data.split(":")[0]

    CALLBACK_ROUTES = {
        "settings": handle_settings_callback,
        "order": handle_order_callback,
        "page": handle_pagination,
        "confirm": handle_confirmation,
    }

    handler = CALLBACK_ROUTES.get(prefix, handle_unknown_callback)
    handler(query)
```

## Conversation state machines

For multi-step flows (e.g., forms, wizards):

```python
# States
IDLE = "idle"
AWAITING_NAME = "awaiting_name"
AWAITING_EMAIL = "awaiting_email"
AWAITING_CONFIRM = "awaiting_confirm"

# State storage (Redis, DB, or in-memory dict)
user_states = {}  # {user_id: {"state": ..., "data": {...}}}

def handle_message(update):
    user_id = update.message.from_user.id
    state = user_states.get(user_id, {}).get("state", IDLE)

    HANDLERS = {
        IDLE: handle_idle,
        AWAITING_NAME: handle_name_input,
        AWAITING_EMAIL: handle_email_input,
        AWAITING_CONFIRM: handle_confirmation,
    }

    HANDLERS[state](update)

def handle_idle(update):
    if update.message.text == "/register":
        user_states[update.message.from_user.id] = {
            "state": AWAITING_NAME, "data": {}
        }
        send("What's your name?")

def handle_name_input(update):
    uid = update.message.from_user.id
    user_states[uid]["data"]["name"] = update.message.text
    user_states[uid]["state"] = AWAITING_EMAIL
    send("What's your email?")
```

**Tips:**
- Always handle `/cancel` in every state to reset.
- Set timeouts on state (clear after 10-30 minutes of inactivity).
- Store state in Redis or DB for persistence across restarts.

## Middleware pattern

Stack middleware for cross-cutting concerns:

```python
# Middleware chain
def auth_middleware(handler):
    def wrapper(update):
        user_id = update.effective_user.id
        if not is_authorized(user_id):
            send(user_id, "Unauthorized")
            return
        handler(update)
    return wrapper

def rate_limit_middleware(handler, max_per_minute=20):
    counters = {}
    def wrapper(update):
        user_id = update.effective_user.id
        now = time.time()
        # Clean old entries, check rate
        if over_limit(counters, user_id, now, max_per_minute):
            send(user_id, "Too many requests. Try again later.")
            return
        handler(update)
    return wrapper

def logging_middleware(handler):
    def wrapper(update):
        log.info(f"Update from {update.effective_user.id}: {update}")
        try:
            handler(update)
        except Exception as e:
            log.error(f"Handler error: {e}")
            send(update.effective_chat.id, "Something went wrong.")
    return wrapper

# Compose: logging wraps rate_limit wraps auth wraps handler
handler = logging_middleware(rate_limit_middleware(auth_middleware(my_handler)))
```

## Error handling

```python
def safe_handler(update):
    try:
        process(update)
    except TelegramAPIError as e:
        if e.status == 403:
            # User blocked bot — mark inactive
            mark_user_inactive(update.effective_user.id)
        elif e.status == 429:
            # Rate limited — retry after delay
            time.sleep(e.retry_after)
            process(update)
        else:
            log.error(f"API error: {e}")
    except Exception as e:
        log.error(f"Unexpected error: {e}", exc_info=True)
        try:
            send(update.effective_chat.id, "Something went wrong. Try again.")
        except:
            pass  # If we can't even send an error message, just log it
```

## Project structure (recommended)

```
bot/
  __init__.py
  main.py           — Entry point, webhook/polling setup
  config.py         — Environment variables, settings
  handlers/
    __init__.py
    start.py        — /start, /help
    commands.py     — Feature commands
    callbacks.py    — Inline keyboard callbacks
    errors.py       — Error handler
  middleware/
    auth.py         — Authorization checks
    rate_limit.py   — Rate limiting
    logging.py      — Request logging
  services/
    user.py         — User management
    payment.py      — Stars/payment logic
  models/
    user.py         — User data model
    state.py        — Conversation state
  utils/
    keyboard.py     — Keyboard builders
    formatting.py   — Message formatting helpers
```
INNEREOF
echo "  Created notes/architecture.md"
fi

# --- notes/deployment.md ---
if [ ! -f "$VAULT_PATH/notes/deployment.md" ]; then
cat > "$VAULT_PATH/notes/deployment.md" << 'INNEREOF'
# Deployment & Hosting

## Webhook setup

```bash
# Set webhook
curl -X POST "https://api.telegram.org/bot<TOKEN>/setWebhook" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://bot.example.com/webhook/SECRET_PATH",
    "max_connections": 40,
    "allowed_updates": ["message", "callback_query", "pre_checkout_query"],
    "secret_token": "your_random_secret_here"
  }'

# Verify webhook
curl "https://api.telegram.org/bot<TOKEN>/getWebhookInfo"

# Remove webhook (switch to polling)
curl "https://api.telegram.org/bot<TOKEN>/deleteWebhook"
```

**Requirements**: HTTPS with valid SSL cert. Ports: 443, 80, 88, or 8443.

Verify incoming requests:
```python
# Check X-Telegram-Bot-Api-Secret-Token header
if request.headers.get("X-Telegram-Bot-Api-Secret-Token") != SECRET_TOKEN:
    return 403
```

## Dockerfile

```dockerfile
FROM python:3.12-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

# Non-root user
RUN useradd -m botuser
USER botuser

EXPOSE 8443

CMD ["python", "-m", "bot.main"]
```

For Go bots:

```dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bot ./cmd/bot

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /bot /bot
USER nobody
EXPOSE 8443
ENTRYPOINT ["/bot"]
```

## Fly.io

```bash
# fly.toml
fly launch --name my-telegram-bot
fly secrets set BOT_TOKEN=123:ABC WEBHOOK_SECRET=random_string

# fly.toml
[env]
  PORT = "8443"
  WEBHOOK_URL = "https://my-telegram-bot.fly.dev/webhook"

[http_service]
  internal_port = 8443
  force_https = true

[[vm]]
  size = "shared-cpu-1x"
  memory = "256mb"
```

**Pros**: Free tier, auto-sleep, easy deploys, built-in TLS.
**Cons**: Cold starts if using auto-sleep (set `min_machines_running = 1` to avoid).

## Railway

```bash
# Deploy from GitHub repo
railway init
railway up

# Set environment variables in dashboard or CLI
railway variables set BOT_TOKEN=123:ABC
```

**Pros**: Dead-simple deploys, GitHub integration, generous free tier.
**Cons**: Less control over infrastructure.

## VPS (DigitalOcean, Hetzner, etc.)

```bash
# Setup with systemd
sudo cat > /etc/systemd/system/telegram-bot.service << 'EOF'
[Unit]
Description=Telegram Bot
After=network.target

[Service]
User=botuser
WorkingDirectory=/opt/bot
ExecStart=/opt/bot/bot
Restart=always
RestartSec=5
Environment=BOT_TOKEN=123:ABC
EnvironmentFile=/opt/bot/.env

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl enable telegram-bot
sudo systemctl start telegram-bot
```

**SSL with Caddy** (automatic HTTPS):
```
# Caddyfile
bot.example.com {
    reverse_proxy localhost:8443
}
```

## Monitoring

### Health check endpoint

```python
@app.get("/health")
def health():
    return {"status": "ok", "uptime": time.time() - START_TIME}
```

### Webhook status monitoring

```bash
# Cron job: check webhook health every 5 minutes
*/5 * * * * curl -s "https://api.telegram.org/bot$TOKEN/getWebhookInfo" | \
  jq -r '.result | "pending: \(.pending_update_count), errors: \(.last_error_message // "none")"'
```

Key fields in `getWebhookInfo`:
- `pending_update_count` — if consistently > 0, bot is falling behind
- `last_error_date` / `last_error_message` — webhook delivery failures
- `max_connections` — concurrent connections Telegram will use

### Logging best practices

```python
import logging

logging.basicConfig(
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    level=logging.INFO,
)
logger = logging.getLogger("bot")

# Log every update (but not sensitive data)
logger.info(f"update user={update.effective_user.id} type={update_type}")

# Structured logging for production (JSON)
# Use python-json-logger or structlog
```

### Alerting

- Monitor `pending_update_count` — alert if > 100 for 5+ minutes.
- Monitor process restarts — alert if > 3 restarts in 10 minutes.
- Monitor error rate — alert if > 10% of updates fail.
- Use uptime monitoring (UptimeRobot, Betterstack) on `/health` endpoint.
INNEREOF
echo "  Created notes/deployment.md"
fi
