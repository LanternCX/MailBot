"""
core/bot.py
~~~~~~~~~~~
Telegram Bot command and callback handler with long-polling.

Responsibilities:
- Poll for Telegram updates (commands + callback queries)
- Handle /mode command: send inline keyboard mode panel
- Handle /ai command: reply-based AI analysis
- Handle callback queries: mode switching + hybrid summary generation
- Manage global operation mode state
"""

from __future__ import annotations

import logging
import threading
import time
from typing import Any

from core.ai import analyze_email
from core.models import (
    AIAnalysisResult,
    AIConfig,
    EmailSnapshot,
    OperationMode,
)
from core.notifiers.telegram import TelegramNotifier

logger = logging.getLogger("mailbot.bot")


class TelegramBotHandler:
    """
    Handles Telegram Bot updates: commands (/mode, /ai) and callback queries.

    Runs a long-polling loop on a separate daemon thread.
    """

    def __init__(
        self,
        notifier: TelegramNotifier,
        ai_config: AIConfig,
        default_mode: OperationMode = OperationMode.HYBRID,
    ) -> None:
        self._notifier = notifier
        self._ai_config = ai_config

        # Global operation mode (thread-safe via lock)
        self._mode = default_mode
        self._mode_lock = threading.Lock()

        # Polling state
        self._offset: int | None = None
        self._thread: threading.Thread | None = None
        self._stop_event = threading.Event()

        # Cache: uid -> EmailSnapshot body for hybrid callback
        self._email_cache: dict[str, EmailSnapshot] = {}
        self._cache_lock = threading.Lock()

    # ──────────────────────────────────────────────
    #  Mode property
    # ──────────────────────────────────────────────

    @property
    def mode(self) -> OperationMode:
        with self._mode_lock:
            return self._mode

    @mode.setter
    def mode(self, value: OperationMode) -> None:
        with self._mode_lock:
            self._mode = value

    # ──────────────────────────────────────────────
    #  Email cache (for hybrid callback)
    # ──────────────────────────────────────────────

    def cache_email(self, snapshot: EmailSnapshot) -> None:
        """Store email snapshot for later AI callback."""
        with self._cache_lock:
            self._email_cache[snapshot.uid] = snapshot
            # Limit cache size to prevent memory leak
            if len(self._email_cache) > 200:
                oldest = list(self._email_cache.keys())[:100]
                for key in oldest:
                    self._email_cache.pop(key, None)

    def _get_cached_email(self, uid: str) -> EmailSnapshot | None:
        with self._cache_lock:
            return self._email_cache.get(uid)

    # ──────────────────────────────────────────────
    #  Lifecycle
    # ──────────────────────────────────────────────

    def start(self) -> None:
        """Start the polling thread."""
        if self._thread and self._thread.is_alive():
            logger.warning("Bot handler already running")
            return

        self._register_commands()

        self._stop_event.clear()
        self._thread = threading.Thread(
            target=self._poll_loop,
            name="TelegramBot-Poller",
            daemon=True,
        )
        self._thread.start()
        logger.info("Telegram bot handler started (mode=%s)", self._mode.value)

    def stop(self) -> None:
        """Stop the polling thread."""
        self._stop_event.set()
        if self._thread and self._thread.is_alive():
            self._thread.join(timeout=10)
        logger.info("Telegram bot handler stopped")

    # ──────────────────────────────────────────────
    #  Polling loop
    # ──────────────────────────────────────────────

    def _poll_loop(self) -> None:
        """Long-polling loop for Telegram updates."""
        logger.info("Bot polling loop started")

        while not self._stop_event.is_set():
            try:
                updates = self._notifier.get_updates(
                    offset=self._offset,
                    timeout=30,
                )

                for update in updates:
                    update_id = update.get("update_id", 0)
                    self._offset = update_id + 1

                    try:
                        self._handle_update(update)
                    except Exception:
                        logger.exception("Error handling update %d", update_id)

            except Exception:
                logger.exception("Error in bot polling loop")
                # Brief sleep before retry on error
                if not self._stop_event.is_set():
                    time.sleep(5)

        logger.info("Bot polling loop exited")

    def _register_commands(self) -> None:
        """Register bot commands with Telegram for native client help."""
        commands = [
            {"command": "mode", "description": "Open mode switch panel"},
            {"command": "ai", "description": "Reply to a message to analyze with AI"},
            {"command": "help", "description": "Show help"},
        ]
        ok = self._notifier.set_bot_commands(commands)
        if ok:
            logger.info("Telegram bot commands registered")
        else:
            logger.warning("Failed to register Telegram bot commands")

    # ──────────────────────────────────────────────
    #  Update dispatcher
    # ──────────────────────────────────────────────

    def _handle_update(self, update: dict[str, Any]) -> None:
        """Route update to appropriate handler."""
        if "callback_query" in update:
            self._handle_callback_query(update["callback_query"])
        elif "message" in update:
            self._handle_message(update["message"])

    # ──────────────────────────────────────────────
    #  Command handlers
    # ──────────────────────────────────────────────

    def _handle_message(self, message: dict[str, Any]) -> None:
        """Handle incoming messages (commands)."""
        text = message.get("text", "")
        chat_id = str(message.get("chat", {}).get("id", ""))

        if not text or not chat_id:
            return

        # Only process commands
        if text.startswith("/mode"):
            self._cmd_mode(chat_id)
        elif text.startswith("/ai"):
            self._cmd_ai(message, chat_id)
        elif text.startswith("/help") or text.startswith("/start"):
            self._cmd_help(chat_id)

    def _cmd_mode(self, chat_id: str) -> None:
        """Handle /mode command: send inline keyboard panel."""
        logger.info("/mode command from chat %s", chat_id)
        self._notifier.send_mode_panel(self.mode, chat_id=chat_id)

    def _cmd_ai(self, message: dict[str, Any], chat_id: str) -> None:
        """Handle /ai command: must reply to a message, then analyze it."""
        reply = message.get("reply_to_message")
        if not reply:
            # Not a reply — send usage hint
            self._notifier._api_call("sendMessage", {
                "chat_id": chat_id,
                "text": "⚠️ Please reply to a message before using /ai to analyze it.",
                "parse_mode": "HTML",
            })
            return

        reply_text = reply.get("text", "")
        if not reply_text:
            self._notifier._api_call("sendMessage", {
                "chat_id": chat_id,
                "text": "⚠️ The replied message has no text to analyze.",
                "parse_mode": "HTML",
            })
            return

        reply_message_id = reply.get("message_id", 0)

        logger.info("/ai command — analyzing reply message %d", reply_message_id)

        # Force AI analysis regardless of current mode
        result = analyze_email(
            subject="(Manual Analysis)",
            sender="",
            body=reply_text,
            config=self._ai_config,
        )

        self._notifier.send_ai_result(
            chat_id=chat_id,
            reply_to_message_id=reply_message_id,
            result=result,
        )

    def _cmd_help(self, chat_id: str) -> None:
        """Send a short help message with available commands."""
        lines = [
            "🤖 MailBot commands:",
            "/mode — open mode switch panel",
            "/ai — reply to a message to analyze with AI",
            "/help — show this help",
        ]
        text = "\n".join(lines)

        self._notifier._api_call("sendMessage", {
            "chat_id": chat_id,
            "text": text,
            "parse_mode": "HTML",
            "disable_web_page_preview": True,
        })

    # ──────────────────────────────────────────────
    #  Callback query handler
    # ──────────────────────────────────────────────

    def _handle_callback_query(self, cq: dict[str, Any]) -> None:
        """Handle inline keyboard callback queries."""
        cq_id = cq.get("id", "")
        data = cq.get("data", "")
        message = cq.get("message", {})
        chat_id = str(message.get("chat", {}).get("id", ""))
        message_id = message.get("message_id", 0)

        if not data:
            return

        if data.startswith("mode_"):
            self._cb_mode_switch(cq_id, data, chat_id, message_id)
        elif data.startswith("summ_"):
            self._cb_summary(cq_id, data, chat_id, message_id)
        else:
            self._notifier.answer_callback_query(cq_id, "Unknown action")

    def _cb_mode_switch(
        self,
        cq_id: str,
        data: str,
        chat_id: str,
        message_id: int,
    ) -> None:
        """Handle mode switch callback: mode_raw / mode_hybrid / mode_agent."""
        mode_str = data.replace("mode_", "")

        try:
            new_mode = OperationMode(mode_str)
        except ValueError:
            self._notifier.answer_callback_query(cq_id, "Invalid mode")
            return

        old_mode = self.mode
        if new_mode == old_mode:
            self._notifier.answer_callback_query(cq_id, f"Already in {new_mode.value} mode")
            return

        self.mode = new_mode
        logger.info("Mode switched: %s → %s", old_mode.value, new_mode.value)

        # Edit the mode panel message in place
        self._notifier.edit_mode_panel(chat_id, message_id, new_mode)

        # Toast notification
        mode_labels = {
            OperationMode.RAW: "Raw (forward only)",
            OperationMode.HYBRID: "Hybrid (on-demand AI)",
            OperationMode.AGENT: "Agent (always AI)",
        }
        self._notifier.answer_callback_query(
            cq_id,
            f"Switched to {mode_labels.get(new_mode, new_mode.value)}",
        )

    def _cb_summary(
        self,
        cq_id: str,
        data: str,
        chat_id: str,
        message_id: int,
    ) -> None:
        """Handle hybrid mode AI summary callback: summ_{uid}."""
        uid = data.replace("summ_", "")

        # Toast: generating
        self._notifier.answer_callback_query(cq_id, "Generating AI summary…")

        snapshot = self._get_cached_email(uid)
        if not snapshot:
            logger.warning("No cached email for uid=%s", uid)
            self._notifier.edit_message_text(
                chat_id,
                message_id,
                "⚠️ Email cache expired; cannot generate summary.",
            )
            return

        # Run AI analysis
        result = analyze_email(
            subject=snapshot.subject,
            sender=snapshot.sender,
            body=snapshot.body_text,
            config=self._ai_config,
        )

        # Build updated message with summary prepended
        from core.notifiers.telegram import CATEGORY_ICONS, PRIORITY_LABELS

        cat_icon = CATEGORY_ICONS.get(result.category, "📧")
        pri_label = PRIORITY_LABELS.get(result.priority, "🟡 Medium")

        esc = TelegramNotifier._escape_html

        lines = [
            f"🤖 <b>AI Summary</b>  {cat_icon} {esc(result.category)}  |  {pri_label}",
            f"💡 {esc(result.summary)}",
        ]
        if result.extracted_code:
            lines.append(f"🔑 Code: <code>{esc(result.extracted_code)}</code>")

        lines.append("")
        lines.append("─" * 20)
        lines.append(f"📬 <b>Original mail</b>")
        lines.append(f"📧 Account: {esc(snapshot.account_name)}")
        lines.append(f"👤 From: {esc(snapshot.sender)}")
        lines.append(f"📌 Subject: {esc(snapshot.subject)}")

        if snapshot.date:
            lines.append(f"🕐 Time: {snapshot.date.strftime('%Y-%m-%d %H:%M')}")

        preview = snapshot.body_text[:300]
        if len(snapshot.body_text) > 300:
            preview += "…"
        lines.append(f"\n📝 Preview:\n{esc(preview)}")

        if snapshot.web_link:
            lines.append(f"\n🔗 <a href=\"{snapshot.web_link}\">Open in webmail</a>")

        text = "\n".join(lines)

        # Edit original message — remove the AI button
        self._notifier.edit_message_text(chat_id, message_id, text, parse_mode="HTML")
