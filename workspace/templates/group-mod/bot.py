"""
SameVault Group Moderator Bot

A Telegram bot that monitors group chat messages using AI to detect spam,
toxicity, and off-topic content. Uses SAME to track repeat offenders and
can warn, mute, or ban users based on their history.

Environment variables:
    TELEGRAM_BOT_TOKEN  — Bot token from @BotFather
    OPENROUTER_API_KEY  — API key from openrouter.ai
    SAME_API_URL        — SAME memory endpoint (default: local CLI)
    VAULT_PATH          — Path to SAME vault (default: /data/vault)
    GROUP_TOPIC         — Description of the group's topic (for off-topic detection)
    WARN_THRESHOLD      — Warnings before auto-mute (default: 3)
    MUTE_DURATION       — Mute duration in minutes (default: 60)
"""

import json
import logging
import os
import sys
from datetime import datetime, timedelta, timezone

import httpx
from telegram import ChatPermissions, Update
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
GROUP_TOPIC = os.environ.get(
    "GROUP_TOPIC", "general discussion"
)
WARN_THRESHOLD = int(os.environ.get("WARN_THRESHOLD", "3"))
MUTE_DURATION = int(os.environ.get("MUTE_DURATION", "60"))

MODEL = "openai/gpt-4o-mini"

logging.basicConfig(
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
    level=logging.INFO,
)
logger = logging.getLogger(__name__)

# SAME vault instance
vault = SameVault(VAULT_PATH)


# ---------------------------------------------------------------------------
# Offender Tracking
# ---------------------------------------------------------------------------


def get_warning_count(user_id: int) -> int:
    """Count how many warnings a user has received."""
    results = vault.search(f"WARNING user:{user_id}", limit=50)
    count = 0
    for r in results:
        text = r.get("text", "")
        if f"user:{user_id}" in text and "WARNING:" in text:
            count += 1
    return count


def record_warning(
    user_id: int, username: str, reason: str, message_text: str
) -> int:
    """Record a warning and return the new warning count."""
    timestamp = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")
    vault.add(
        f"WARNING: [{timestamp}] user:{user_id} ({username}) — {reason}. "
        f"Message: {message_text[:200]}",
        tags=["warning", f"user:{user_id}", reason],
        source="group-mod",
    )
    return get_warning_count(user_id)


def record_action(user_id: int, username: str, action: str):
    """Record a moderation action (mute, ban) for audit trail."""
    timestamp = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")
    vault.add(
        f"ACTION: [{timestamp}] {action} user:{user_id} ({username})",
        tags=["mod-action", action, f"user:{user_id}"],
        source="group-mod",
    )


# ---------------------------------------------------------------------------
# AI Moderation
# ---------------------------------------------------------------------------

MODERATION_PROMPT = """You are a group chat moderator AI. Analyze the following message and classify it.

Group topic: {topic}

Respond with a JSON object (and NOTHING else) containing:
- "spam": boolean — is this spam or self-promotion?
- "toxic": boolean — is this abusive, hateful, or harassing?
- "off_topic": boolean — is this unrelated to the group's topic?
- "reason": string — brief explanation (1 sentence) if any flag is true, otherwise empty string
- "severity": string — "none", "low", "medium", or "high"

Only flag genuinely problematic content. Normal conversation, questions,
and friendly banter should pass. Err on the side of permissiveness.

Message to analyze:
{message}"""


async def analyze_message(text: str) -> dict:
    """Use AI to classify a message for moderation."""
    prompt = MODERATION_PROMPT.format(topic=GROUP_TOPIC, message=text)

    async with httpx.AsyncClient(timeout=30.0) as client:
        response = await client.post(
            "https://openrouter.ai/api/v1/chat/completions",
            headers={
                "Authorization": f"Bearer {OPENROUTER_API_KEY}",
                "Content-Type": "application/json",
            },
            json={
                "model": MODEL,
                "messages": [
                    {"role": "user", "content": prompt},
                ],
                "max_tokens": 256,
                "temperature": 0.1,  # Low temp for consistent classification
            },
        )
        response.raise_for_status()
        data = response.json()

    content = data["choices"][0]["message"]["content"]

    # Parse JSON from the response, handling markdown code blocks
    content = content.strip()
    if content.startswith("```"):
        # Strip markdown code fence
        lines = content.split("\n")
        content = "\n".join(lines[1:-1])

    try:
        return json.loads(content)
    except json.JSONDecodeError:
        logger.warning("Failed to parse moderation response: %s", content)
        return {
            "spam": False,
            "toxic": False,
            "off_topic": False,
            "reason": "",
            "severity": "none",
        }


# ---------------------------------------------------------------------------
# Telegram Handlers
# ---------------------------------------------------------------------------


async def cmd_start(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Handle /start — show bot info."""
    await update.message.reply_text(
        "I'm a group moderator bot. Add me to your group and give me "
        "admin permissions to get started.\n\n"
        "I monitor messages for spam, toxicity, and off-topic content. "
        "Repeat offenders get warnings, then muted.\n\n"
        "Admin commands (group admins only):\n"
        "  /modstats — show moderation statistics\n"
        "  /history <user_id> — show a user's warning history\n"
    )


async def cmd_modstats(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Handle /modstats — show moderation statistics."""
    # Only allow group admins
    if update.effective_chat.type in ("group", "supergroup"):
        member = await update.effective_chat.get_member(
            update.effective_user.id
        )
        if member.status not in ("administrator", "creator"):
            await update.message.reply_text("This command is for admins only.")
            return

    warnings = vault.search("WARNING:", limit=50)
    actions = vault.search("ACTION:", limit=50)

    warning_count = sum(
        1
        for w in warnings
        if "WARNING:" in w.get("text", "")
    )
    action_count = sum(
        1
        for a in actions
        if "ACTION:" in a.get("text", "")
    )

    await update.message.reply_text(
        f"Moderation Stats:\n"
        f"  Warnings issued: {warning_count}\n"
        f"  Actions taken: {action_count}\n"
        f"  Warn threshold: {WARN_THRESHOLD}\n"
        f"  Mute duration: {MUTE_DURATION} min\n"
        f"  Group topic: {GROUP_TOPIC}"
    )


async def cmd_history(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Handle /history — show warning history for a user."""
    if update.effective_chat.type in ("group", "supergroup"):
        member = await update.effective_chat.get_member(
            update.effective_user.id
        )
        if member.status not in ("administrator", "creator"):
            await update.message.reply_text("This command is for admins only.")
            return

    if not context.args:
        await update.message.reply_text("Usage: /history <user_id>")
        return

    target = context.args[0].lstrip("@")
    results = vault.search(f"WARNING: user:{target}", limit=20)
    warnings = [
        r.get("text", "")
        for r in results
        if "WARNING:" in r.get("text", "")
    ]

    if warnings:
        history_text = "\n".join(f"  {w}" for w in warnings[:10])
        await update.message.reply_text(
            f"Warning history ({len(warnings)} total):\n{history_text}"
        )
    else:
        await update.message.reply_text("No warnings found for this user.")


async def handle_group_message(
    update: Update, context: ContextTypes.DEFAULT_TYPE
):
    """
    Monitor group messages for violations.

    Flow:
    1. AI analyzes the message
    2. If flagged, record a warning in SAME
    3. If warnings exceed threshold, mute the user
    4. Delete messages flagged as high severity
    """
    message = update.message
    if not message or not message.text:
        return

    user = update.effective_user
    user_id = user.id
    username = user.username or user.first_name or str(user_id)

    # Don't moderate bot messages or admin messages
    try:
        member = await update.effective_chat.get_member(user_id)
        if member.status in ("administrator", "creator"):
            return
    except Exception:
        pass  # If we can't check, proceed with moderation

    # Analyze the message
    try:
        analysis = await analyze_message(message.text)
    except Exception as e:
        logger.error("Moderation analysis failed: %s", e)
        return  # Fail open — don't block messages if AI is unavailable

    flagged = analysis.get("spam") or analysis.get("toxic") or analysis.get("off_topic")

    if not flagged:
        return  # Message is clean

    # Determine the violation type
    violations = []
    if analysis.get("spam"):
        violations.append("spam")
    if analysis.get("toxic"):
        violations.append("toxic")
    if analysis.get("off_topic"):
        violations.append("off-topic")

    reason = ", ".join(violations)
    severity = analysis.get("severity", "low")
    explanation = analysis.get("reason", "")

    logger.info(
        "Flagged message from %s (%d): %s [%s] — %s",
        username, user_id, reason, severity, explanation,
    )

    # Record warning in SAME
    warning_count = record_warning(user_id, username, reason, message.text)

    # Delete high-severity messages (spam, toxic)
    if severity == "high":
        try:
            await message.delete()
            logger.info("Deleted high-severity message from %s", username)
        except Exception as e:
            logger.warning("Could not delete message: %s", e)

    # Take action based on warning count
    if warning_count >= WARN_THRESHOLD:
        # Mute the user
        mute_until = datetime.now(timezone.utc) + timedelta(minutes=MUTE_DURATION)
        try:
            await update.effective_chat.restrict_member(
                user_id,
                permissions=ChatPermissions(can_send_messages=False),
                until_date=mute_until,
            )
            record_action(user_id, username, "mute")
            await update.effective_chat.send_message(
                f"@{username} has been muted for {MUTE_DURATION} minutes. "
                f"Reason: {warning_count} warnings ({reason})."
            )
            logger.info("Muted %s for %d minutes", username, MUTE_DURATION)
        except Exception as e:
            logger.warning("Could not mute user: %s", e)
    elif severity != "high":
        # Issue a warning (don't warn if we already deleted the message)
        try:
            await message.reply_text(
                f"Warning ({warning_count}/{WARN_THRESHOLD}): "
                f"Your message was flagged for {reason}. "
                f"{explanation}\n\n"
                f"After {WARN_THRESHOLD} warnings you'll be temporarily muted."
            )
        except Exception as e:
            logger.warning("Could not send warning: %s", e)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def main():
    """Start the moderation bot."""
    logger.info("Starting Group Moderator bot...")
    logger.info("Vault path: %s", VAULT_PATH)
    logger.info("Group topic: %s", GROUP_TOPIC)
    logger.info("Warn threshold: %d, Mute duration: %d min", WARN_THRESHOLD, MUTE_DURATION)

    app = Application.builder().token(TELEGRAM_BOT_TOKEN).build()

    # Command handlers
    app.add_handler(CommandHandler("start", cmd_start))
    app.add_handler(CommandHandler("modstats", cmd_modstats))
    app.add_handler(CommandHandler("history", cmd_history))

    # Monitor all group text messages
    app.add_handler(
        MessageHandler(
            filters.TEXT & ~filters.COMMAND & (
                filters.ChatType.GROUP | filters.ChatType.SUPERGROUP
            ),
            handle_group_message,
        )
    )

    logger.info("Bot is polling for updates...")
    app.run_polling(drop_pending_updates=True)


if __name__ == "__main__":
    main()
