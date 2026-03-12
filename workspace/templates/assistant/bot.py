"""
SameVault Personal AI Assistant Bot

A Telegram bot that uses AI (via OpenRouter) for conversation and SAME
for persistent memory. The bot remembers user preferences, facts, and
conversation context across sessions.

Environment variables:
    TELEGRAM_BOT_TOKEN  — Bot token from @BotFather
    OPENROUTER_API_KEY  — API key from openrouter.ai
    SAME_API_URL        — SAME memory endpoint (default: local CLI)
    VAULT_PATH          — Path to SAME vault (default: /data/vault)
"""

import logging
import os
import subprocess
import sys
from datetime import datetime, timezone

import httpx
from telegram import Update
from telegram.ext import (
    Application,
    CommandHandler,
    ContextTypes,
    MessageHandler,
    filters,
)

# same_sdk lives in the parent directory (templates/) during development,
# and in the working directory (/app/) when deployed via Docker.
_here = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(_here, ".."))  # templates/
sys.path.insert(0, _here)                       # /app/ (Docker)
from same_sdk import SameVault

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

TELEGRAM_BOT_TOKEN = os.environ["TELEGRAM_BOT_TOKEN"]
OPENROUTER_API_KEY = os.environ["OPENROUTER_API_KEY"]
VAULT_PATH = os.environ.get("VAULT_PATH", "/data/vault")
SAME_API_URL = os.environ.get("SAME_API_URL", "")

# Default model — can be changed per-user with /model
DEFAULT_MODEL = "openai/gpt-4o-mini"

# How many memories to retrieve for context
MEMORY_CONTEXT_LIMIT = 10

logging.basicConfig(
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
    level=logging.INFO,
)
logger = logging.getLogger(__name__)

# Per-user model selection (in-memory; for persistence, store in SAME)
user_models: dict[int, str] = {}

# SAME vault instance — shared across all handlers
vault = SameVault(VAULT_PATH)


# ---------------------------------------------------------------------------
# AI (OpenRouter) Helpers
# ---------------------------------------------------------------------------


def build_system_prompt(memories: list[dict]) -> str:
    """Build a system prompt that includes relevant memories for context."""
    base = (
        "You are a helpful personal assistant with persistent memory. "
        "You remember things the user has told you in past conversations. "
        "Be concise, friendly, and helpful."
    )
    if not memories:
        return base

    memory_lines = []
    for mem in memories:
        text = mem.get("text", "")
        if text:
            memory_lines.append(f"- {text}")

    if memory_lines:
        base += (
            "\n\nHere are relevant things you remember about this user:\n"
            + "\n".join(memory_lines)
        )
    return base


async def ask_ai(
    user_message: str, memories: list[dict], model: str
) -> str:
    """Send a message to OpenRouter and return the AI response."""
    system_prompt = build_system_prompt(memories)

    async with httpx.AsyncClient(timeout=60.0) as client:
        response = await client.post(
            "https://openrouter.ai/api/v1/chat/completions",
            headers={
                "Authorization": f"Bearer {OPENROUTER_API_KEY}",
                "Content-Type": "application/json",
            },
            json={
                "model": model,
                "messages": [
                    {"role": "system", "content": system_prompt},
                    {"role": "user", "content": user_message},
                ],
                "max_tokens": 1024,
            },
        )
        response.raise_for_status()
        data = response.json()

    return data["choices"][0]["message"]["content"]


# ---------------------------------------------------------------------------
# Telegram Command Handlers
# ---------------------------------------------------------------------------


async def cmd_start(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Handle /start — greet the user."""
    await update.message.reply_text(
        "Hi! I'm your personal AI assistant with persistent memory.\n\n"
        "Just send me a message and I'll respond. I remember our "
        "conversations across sessions.\n\n"
        "Commands:\n"
        "  /model <name> — switch AI model\n"
        "  /forget — clear all my memories\n"
        "  /help — show this message"
    )


async def cmd_help(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Handle /help — show available commands."""
    await update.message.reply_text(
        "Commands:\n"
        "  /model <name> — switch AI model (e.g. openai/gpt-4o)\n"
        "  /model — show current model\n"
        "  /forget — erase all my memories of you\n"
        "  /help — this message"
    )


async def cmd_model(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Handle /model — view or change the AI model."""
    user_id = update.effective_user.id
    current = user_models.get(user_id, DEFAULT_MODEL)

    if context.args:
        new_model = context.args[0]
        user_models[user_id] = new_model
        await update.message.reply_text(f"Model switched to: {new_model}")
    else:
        await update.message.reply_text(
            f"Current model: {current}\n\n"
            "To change: /model <model-name>\n"
            "Examples:\n"
            "  /model openai/gpt-4o\n"
            "  /model anthropic/claude-3.5-sonnet\n"
            "  /model google/gemini-pro"
        )


async def cmd_forget(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Handle /forget — clear all memories."""
    try:
        # Reinitialize the vault to clear all data
        result = subprocess.run(
            ["same", "init", "--path", VAULT_PATH, "--force"],
            capture_output=True, text=True, timeout=10,
        )
        if result.returncode == 0:
            await update.message.reply_text(
                "All memories cleared. Starting fresh."
            )
        else:
            await update.message.reply_text(
                "Something went wrong clearing memories. Check the logs."
            )
    except Exception as e:
        logger.error("Forget failed: %s", e)
        await update.message.reply_text(
            "Something went wrong clearing memories. Check the logs."
        )


async def handle_message(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Handle regular text messages — the main conversation loop."""
    user_id = update.effective_user.id
    user_message = update.message.text
    model = user_models.get(user_id, DEFAULT_MODEL)
    username = update.effective_user.first_name or "User"

    # 1. Recall relevant memories for context
    memories = vault.search(user_message, limit=MEMORY_CONTEXT_LIMIT)

    # 2. Get AI response with memory context
    try:
        response = await ask_ai(user_message, memories, model)
    except Exception as e:
        logger.error("AI request failed: %s", e)
        await update.message.reply_text(
            "Sorry, I had trouble reaching the AI service. Please try again."
        )
        return

    # 3. Store the exchange in memory for future context
    timestamp = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")
    vault.add(
        f"[{timestamp}] {username}: {user_message}",
        tags=["conversation", f"user:{user_id}"],
        source="telegram-bot",
    )
    vault.add(
        f"[{timestamp}] Assistant: {response}",
        tags=["conversation", "assistant-reply", f"user:{user_id}"],
        source="telegram-bot",
    )

    # 4. Send the response
    await update.message.reply_text(response)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def main():
    """Start the bot."""
    logger.info("Starting Personal AI Assistant bot...")
    logger.info("Model default: %s", DEFAULT_MODEL)
    logger.info("Vault path: %s", VAULT_PATH)

    app = Application.builder().token(TELEGRAM_BOT_TOKEN).build()

    # Register handlers
    app.add_handler(CommandHandler("start", cmd_start))
    app.add_handler(CommandHandler("help", cmd_help))
    app.add_handler(CommandHandler("model", cmd_model))
    app.add_handler(CommandHandler("forget", cmd_forget))
    app.add_handler(
        MessageHandler(filters.TEXT & ~filters.COMMAND, handle_message)
    )

    # Start polling (long-poll, no webhook needed)
    logger.info("Bot is polling for updates...")
    app.run_polling(drop_pending_updates=True)


if __name__ == "__main__":
    main()
