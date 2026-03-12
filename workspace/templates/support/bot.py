"""
SameVault Customer Support Bot

A Telegram bot that answers questions from a knowledge base stored in SAME.
On startup, it ingests all .md and .txt files from the knowledge/ directory
into the SAME vault. When a user asks a question, it searches the knowledge
base and uses AI to synthesize an answer. If confidence is low, it escalates
to a human support agent.

Environment variables:
    TELEGRAM_BOT_TOKEN  — Bot token from @BotFather
    OPENROUTER_API_KEY  — API key from openrouter.ai
    SAME_API_URL        — SAME memory endpoint (default: local CLI)
    VAULT_PATH          — Path to SAME vault (default: /data/vault)
    ESCALATION_CHAT_ID  — Telegram chat ID to forward escalations to
    CONFIDENCE_THRESHOLD — Minimum sources to answer without escalation (default: 2)
"""

import logging
import os
import sys
from pathlib import Path

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
ESCALATION_CHAT_ID = os.environ.get("ESCALATION_CHAT_ID", "")
CONFIDENCE_THRESHOLD = int(os.environ.get("CONFIDENCE_THRESHOLD", "2"))

MODEL = "openai/gpt-4o-mini"
KNOWLEDGE_DIR = Path(__file__).parent / "knowledge"

logging.basicConfig(
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
    level=logging.INFO,
)
logger = logging.getLogger(__name__)

# SAME vault instance
vault = SameVault(VAULT_PATH)


# ---------------------------------------------------------------------------
# Knowledge Base Ingestion
# ---------------------------------------------------------------------------


def ingest_knowledge():
    """
    Read all .md and .txt files from the knowledge/ directory and store
    each one in SAME. Files are chunked by paragraph so that search
    returns focused results rather than entire documents.
    """
    if not KNOWLEDGE_DIR.exists():
        logger.warning("Knowledge directory not found: %s", KNOWLEDGE_DIR)
        return

    files = list(KNOWLEDGE_DIR.glob("*.md")) + list(KNOWLEDGE_DIR.glob("*.txt"))
    if not files:
        logger.warning("No knowledge files found in %s", KNOWLEDGE_DIR)
        return

    logger.info("Ingesting %d knowledge files from %s", len(files), KNOWLEDGE_DIR)

    for filepath in files:
        content = filepath.read_text(encoding="utf-8").strip()
        if not content:
            continue

        source_name = filepath.name

        # Chunk by double-newline (paragraphs). Keeps search results focused.
        paragraphs = [p.strip() for p in content.split("\n\n") if p.strip()]

        chunk_count = 0
        for paragraph in paragraphs:
            # Skip very short fragments (headings, blank lines)
            if len(paragraph) < 20:
                continue

            vault.add(
                f"[Source: {source_name}] {paragraph}",
                tags=["knowledge", f"source:{source_name}"],
                source=source_name,
                reindex=False,  # Batch mode — reindex once at the end
            )
            chunk_count += 1

        logger.info("  Ingested %s (%d chunks)", source_name, chunk_count)

    # Reindex once after all chunks are added
    vault.reindex()
    logger.info("Knowledge ingestion complete.")


# ---------------------------------------------------------------------------
# AI (OpenRouter) Helpers
# ---------------------------------------------------------------------------


def build_support_prompt(knowledge: list[dict]) -> str:
    """Build a system prompt for support with knowledge context."""
    base = (
        "You are a helpful customer support assistant. Answer the user's "
        "question based ONLY on the knowledge base provided below. "
        "If the knowledge base does not contain enough information to answer "
        "confidently, say so clearly — do not make things up.\n\n"
        "Be concise, professional, and friendly."
    )

    if knowledge:
        kb_lines = []
        for item in knowledge:
            text = item.get("text", "")
            if text:
                kb_lines.append(f"- {text}")

        if kb_lines:
            base += (
                "\n\nRelevant knowledge base entries:\n"
                + "\n".join(kb_lines)
            )
    else:
        base += (
            "\n\nNo relevant knowledge base entries were found. "
            "Let the user know you don't have information on this topic "
            "and offer to escalate to a human agent."
        )

    return base


async def ask_ai(user_message: str, knowledge: list[dict]) -> str:
    """Send a support query to OpenRouter and return the response."""
    system_prompt = build_support_prompt(knowledge)

    async with httpx.AsyncClient(timeout=60.0) as client:
        response = await client.post(
            "https://openrouter.ai/api/v1/chat/completions",
            headers={
                "Authorization": f"Bearer {OPENROUTER_API_KEY}",
                "Content-Type": "application/json",
            },
            json={
                "model": MODEL,
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
# Telegram Handlers
# ---------------------------------------------------------------------------


async def cmd_start(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Handle /start — greet the user."""
    await update.message.reply_text(
        "Hi! I'm a support assistant. Ask me anything and I'll search "
        "our knowledge base to help you.\n\n"
        "If I can't find an answer, I'll offer to connect you with "
        "a human agent.\n\n"
        "Commands:\n"
        "  /escalate — request a human agent\n"
        "  /help — show this message"
    )


async def cmd_help(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Handle /help — show available commands."""
    await update.message.reply_text(
        "Commands:\n"
        "  /escalate — request a human support agent\n"
        "  /help — this message\n\n"
        "Or just send a message with your question."
    )


async def cmd_escalate(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Handle /escalate — forward the conversation to a human."""
    user = update.effective_user
    username = user.username or user.first_name or str(user.id)

    if ESCALATION_CHAT_ID:
        try:
            await context.bot.send_message(
                chat_id=int(ESCALATION_CHAT_ID),
                text=(
                    f"Support escalation from @{username} "
                    f"(ID: {user.id}):\n\n"
                    "User requested human assistance."
                ),
            )
            await update.message.reply_text(
                "I've notified a human agent. They'll be in touch soon."
            )
        except Exception as e:
            logger.error("Escalation failed: %s", e)
            await update.message.reply_text(
                "Sorry, I couldn't reach the support team right now. "
                "Please try again later."
            )
    else:
        await update.message.reply_text(
            "Human escalation is not configured for this bot. "
            "Please contact support directly."
        )


async def handle_message(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Handle incoming questions — search knowledge base and respond."""
    user_message = update.message.text
    user = update.effective_user

    # Search the knowledge base
    results = vault.search(user_message, limit=5)

    # Filter to results that actually have content
    relevant = [r for r in results if r.get("text")]

    # If too few results, flag for potential escalation
    low_confidence = len(relevant) < CONFIDENCE_THRESHOLD

    # Get AI response
    try:
        response = await ask_ai(user_message, relevant)
    except Exception as e:
        logger.error("AI request failed: %s", e)
        await update.message.reply_text(
            "Sorry, I'm having trouble right now. Please try again."
        )
        return

    # Append escalation suggestion if confidence is low
    if low_confidence:
        response += (
            "\n\n---\n"
            "I'm not fully confident in this answer. "
            "Type /escalate to talk to a human agent."
        )

    # Log the interaction in SAME for analytics
    vault.add(
        f"Support Q from user {user.id}: {user_message}",
        tags=["support-query", f"user:{user.id}"],
        source="support-bot",
    )

    await update.message.reply_text(response)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def main():
    """Start the support bot."""
    logger.info("Starting Customer Support bot...")
    logger.info("Vault path: %s", VAULT_PATH)
    logger.info("Knowledge dir: %s", KNOWLEDGE_DIR)

    # Ingest knowledge base into SAME on startup
    ingest_knowledge()

    app = Application.builder().token(TELEGRAM_BOT_TOKEN).build()

    app.add_handler(CommandHandler("start", cmd_start))
    app.add_handler(CommandHandler("help", cmd_help))
    app.add_handler(CommandHandler("escalate", cmd_escalate))
    app.add_handler(
        MessageHandler(filters.TEXT & ~filters.COMMAND, handle_message)
    )

    logger.info("Bot is polling for updates...")
    app.run_polling(drop_pending_updates=True)


if __name__ == "__main__":
    main()
