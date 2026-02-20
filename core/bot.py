"""
core/bot.py
~~~~~~~~~~~
Telegram Bot command and callback handler with long-polling.

Responsibilities:
- Poll for Telegram updates (commands + callback queries)
- Handle /settings command: multi-level inline keyboard dashboard
- Handle /rules command: view/manage persona rules
- Handle /ai command: reply-based AI analysis
- Handle /help command
- Handle callback queries: language/mode switching, hybrid AI summary, settings nav
- Manage global operation mode + language state
- Persist config changes to JSON
"""

from __future__ import annotations

import json
import logging
import threading
import time
from pathlib import Path
from typing import Any

from core.ai import analyze_email
from core.models import (
    AIAnalysisResult,
    AIConfig,
    AppConfig,
    EmailSnapshot,
    OperationMode,
)
from core.notifiers.telegram import TelegramNotifier
from core.rules import RulesManager

logger = logging.getLogger("mailbot.bot")

CONFIG_PATH = Path("config.json")


class TelegramBotHandler:
    """
    Handles Telegram Bot updates: commands and callback queries.

    Runs a long-polling loop on a separate daemon thread.
    """

    def __init__(
        self,
        notifier: TelegramNotifier,
        ai_config: AIConfig,
        default_mode: OperationMode = OperationMode.HYBRID,
        config_path: Path = CONFIG_PATH,
    ) -> None:
        self._notifier = notifier
        self._ai_config = ai_config
        self._config_path = config_path

        # Global operation mode (thread-safe via lock)
        self._mode = default_mode
        self._mode_lock = threading.Lock()

        # Language setting (thread-safe)
        self._language = ai_config.language or "auto"
        self._lang_lock = threading.Lock()

        # AI enabled flag (thread-safe)
        self._ai_enabled = ai_config.enabled
        self._ai_enabled_lock = threading.Lock()

        # Rules manager
        self._rules = RulesManager()

        # Polling state
        self._offset: int | None = None
        self._thread: threading.Thread | None = None
        self._stop_event = threading.Event()

        # Cache: uid -> EmailSnapshot body for hybrid callback
        self._email_cache: dict[str, EmailSnapshot] = {}
        # Cache: uid -> source_language for hybrid mode translate button decision
        self._source_language_cache: dict[str, str | None] = {}
        self._cache_lock = threading.Lock()

        # /rules conversation state: chat_id → "add" | "delete" | None
        self._rules_pending: dict[str, str] = {}
        self._rules_lock = threading.Lock()

    # ──────────────────────────────────────────────
    #  Properties
    # ──────────────────────────────────────────────

    @property
    def mode(self) -> OperationMode:
        with self._mode_lock:
            return self._mode

    @mode.setter
    def mode(self, value: OperationMode) -> None:
        with self._mode_lock:
            self._mode = value

    @property
    def language(self) -> str:
        with self._lang_lock:
            return self._language

    @language.setter
    def language(self, value: str) -> None:
        with self._lang_lock:
            self._language = value

    @property
    def ai_enabled(self) -> bool:
        with self._ai_enabled_lock:
            return self._ai_enabled

    @ai_enabled.setter
    def ai_enabled(self, value: bool) -> None:
        with self._ai_enabled_lock:
            self._ai_enabled = value

    @property
    def rules_block(self) -> str | None:
        """Return formatted rules for prompt injection, or None if empty."""
        return self._rules.as_prompt_block()

    # ──────────────────────────────────────────────
    #  Email cache (for hybrid callback)
    # ──────────────────────────────────────────────

    def cache_email(self, snapshot: EmailSnapshot, source_language: str | None = None) -> None:
        """Store email snapshot and its source language for later AI callback."""
        with self._cache_lock:
            self._email_cache[snapshot.uid] = snapshot
            if source_language:
                self._source_language_cache[snapshot.uid] = source_language
            if len(self._email_cache) > 200:
                oldest = list(self._email_cache.keys())[:100]
                for key in oldest:
                    self._email_cache.pop(key, None)
                    self._source_language_cache.pop(key, None)

    def _get_cached_email(self, uid: str) -> EmailSnapshot | None:
        with self._cache_lock:
            return self._email_cache.get(uid)

    def _get_cached_source_language(self, uid: str) -> str | None:
        with self._cache_lock:
            return self._source_language_cache.get(uid)

    # ──────────────────────────────────────────────
    #  Config persistence
    # ──────────────────────────────────────────────

    def _persist_ai_config(self) -> None:
        """Write current runtime AI settings back to config.json."""
        try:
            if not self._config_path.exists():
                logger.warning("Config file not found, skip persist: %s", self._config_path)
                return
            raw = json.loads(self._config_path.read_text(encoding="utf-8"))
            ai = raw.setdefault("ai", {})
            ai["language"] = self.language
            ai["enabled"] = self.ai_enabled
            ai["default_mode"] = self.mode.value
            self._config_path.write_text(
                json.dumps(raw, indent=2, ensure_ascii=False), encoding="utf-8"
            )
            logger.info("AI config persisted: lang=%s mode=%s ai=%s",
                        self.language, self.mode.value, self.ai_enabled)
        except Exception:
            logger.exception("Failed to persist AI config")

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
        logger.info("Telegram bot handler started (mode=%s, lang=%s)", self._mode.value, self._language)

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
                if not self._stop_event.is_set():
                    time.sleep(5)

        logger.info("Bot polling loop exited")

    def _register_commands(self) -> None:
        """Register bot commands with Telegram for native client help."""
        commands = [
            {"command": "settings", "description": "Open settings dashboard"},
            {"command": "rules", "description": "View / manage persona rules"},
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
        """Handle incoming messages (commands + rules conversation)."""
        text = message.get("text", "")
        chat_id = str(message.get("chat", {}).get("id", ""))

        if not text or not chat_id:
            return

        # Commands take priority
        if text.startswith("/settings") or text.startswith("/mode"):
            self._cmd_settings(chat_id)
            return
        if text.startswith("/rules"):
            self._cmd_rules(chat_id, text)
            return
        if text.startswith("/ai"):
            self._cmd_ai(message, chat_id)
            return
        if text.startswith("/help") or text.startswith("/start"):
            self._cmd_help(chat_id)
            return

        # Check if in /rules conversation
        with self._rules_lock:
            pending = self._rules_pending.pop(chat_id, None)

        if pending == "add":
            self._rules_add(chat_id, text)
        elif pending == "delete":
            self._rules_delete(chat_id, text)

    # ── /settings ──

    def _cmd_settings(self, chat_id: str) -> None:
        """Handle /settings command: send multi-level dashboard."""
        logger.info("/settings command from chat %s", chat_id)
        self._notifier.send_settings_panel(
            current_mode=self.mode,
            language=self.language,
            chat_id=chat_id,
        )

    # ── /rules ──

    def _cmd_rules(self, chat_id: str, text: str) -> None:
        """
        Handle /rules command:
          /rules         → show all rules + action buttons
          /rules add     → enter add mode
          /rules delete  → enter delete mode
        """
        logger.info("/rules command from chat %s", chat_id)
        parts = text.strip().split(maxsplit=1)
        sub = parts[1].strip().lower() if len(parts) > 1 else ""

        if sub == "add":
            with self._rules_lock:
                self._rules_pending[chat_id] = "add"
            self._notifier._api_call("sendMessage", {
                "chat_id": chat_id,
                "text": "📝 Send me the rule text you want to add.\n\nSend /rules to cancel.",
                "parse_mode": "HTML",
            })
            return

        if sub == "delete":
            with self._rules_lock:
                self._rules_pending[chat_id] = "delete"
            self._notifier._api_call("sendMessage", {
                "chat_id": chat_id,
                "text": "🗑 Send me the rule number to delete (e.g. <code>2</code>).\n\nSend /rules to cancel.",
                "parse_mode": "HTML",
            })
            return

        # Default: show rules list
        raw = self._rules.load_raw()
        rules = self._rules.load_rules()
        if not rules:
            msg = "📋 <b>Persona Rules</b>\n\nNo rules yet.\n\nUse /rules add to add a rule."
        else:
            numbered = "\n".join(f"{i}. {r}" for i, r in enumerate(rules, 1))
            msg = (
                f"📋 <b>Persona Rules</b> ({len(rules)})\n\n"
                f"{self._notifier._escape_html(numbered)}\n\n"
                "Use /rules add or /rules delete to manage."
            )

        keyboard = {
            "inline_keyboard": [
                [
                    {"text": "➕ Add", "callback_data": "rules_add"},
                    {"text": "🗑 Delete", "callback_data": "rules_delete"},
                ],
                [
                    {"text": "❌ Close", "callback_data": "rules_close"},
                ],
            ]
        }

        self._notifier._api_call("sendMessage", {
            "chat_id": chat_id,
            "text": msg,
            "parse_mode": "HTML",
            "reply_markup": json.dumps(keyboard),
        })

    def _rules_add(self, chat_id: str, text: str) -> None:
        """Add a rule from user's natural language input."""
        count = self._rules.add_rule(text)
        self._notifier._api_call("sendMessage", {
            "chat_id": chat_id,
            "text": f"✅ Rule added (total: {count}).\n\n<i>{self._notifier._escape_html(text)}</i>",
            "parse_mode": "HTML",
        })
        logger.info("Rule added by chat %s: %s", chat_id, text[:60])

    def _rules_delete(self, chat_id: str, text: str) -> None:
        """Delete a rule by number from user's input."""
        text = text.strip()
        # Try to extract a number
        import re
        match = re.search(r"\d+", text)
        if not match:
            self._notifier._api_call("sendMessage", {
                "chat_id": chat_id,
                "text": "⚠️ Please send a rule number (e.g. <code>2</code>).",
                "parse_mode": "HTML",
            })
            return

        index = int(match.group())
        ok = self._rules.delete_rule(index)
        if ok:
            remaining = len(self._rules.load_rules())
            self._notifier._api_call("sendMessage", {
                "chat_id": chat_id,
                "text": f"✅ Rule #{index} deleted (remaining: {remaining}).",
                "parse_mode": "HTML",
            })
            logger.info("Rule #%d deleted by chat %s", index, chat_id)
        else:
            self._notifier._api_call("sendMessage", {
                "chat_id": chat_id,
                "text": f"⚠️ Rule #{index} not found.",
                "parse_mode": "HTML",
            })

    # ── /ai ──

    def _cmd_ai(self, message: dict[str, Any], chat_id: str) -> None:
        """Handle /ai command: must reply to a message, then analyze it."""
        reply = message.get("reply_to_message")
        if not reply:
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

        # Show typing status
        self._notifier.send_chat_action(chat_id)

        # Build runtime config snapshot with current language
        runtime_config = self._runtime_ai_config()

        result = analyze_email(
            subject="(Manual Analysis)",
            sender="",
            body=reply_text,
            config=runtime_config,
            rules_block=self.rules_block,
        )

        self._notifier.send_ai_result(
            chat_id=chat_id,
            reply_to_message_id=reply_message_id,
            result=result,
        )

    # ── /help ──

    def _cmd_help(self, chat_id: str) -> None:
        """Send a short help message with available commands."""
        lines = [
            "🤖 <b>MailBot</b> commands:",
            "",
            "/settings — open settings dashboard",
            "/rules — view / manage persona rules",
            "/rules add — add a new rule",
            "/rules delete — delete a rule",
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
        """
        Handle inline keyboard callback queries from Telegram.
        
        Routes callbacks to appropriate handlers based on callback_data prefix:
        - settings_* → Dashboard navigation
        - lang_* → Language switching
        - mode_* → Operation mode switching
        - summ_* → AI summary generation
        - orig_* → Show original email
        - trans_* → Translation generation
        - rules_* → Rules management
        """
        cq_id = cq.get("id", "")
        data = cq.get("data", "")
        message = cq.get("message", {})
        chat_id = str(message.get("chat", {}).get("id", ""))
        message_id = message.get("message_id", 0)

        if not data:
            return

        # ── Settings navigation ──
        if data == "settings_lang":
            self._notifier.edit_settings_language_submenu(chat_id, message_id, self.language)
            self._notifier.answer_callback_query(cq_id)
        elif data == "settings_mode":
            self._notifier.edit_settings_mode_submenu(chat_id, message_id, self.mode)
            self._notifier.answer_callback_query(cq_id)
        elif data == "settings_back":
            self._notifier.edit_settings_main(
                chat_id, message_id, self.mode, self.language,
            )
            self._notifier.answer_callback_query(cq_id)
        elif data == "settings_close":
            self._notifier.delete_message(chat_id, message_id)
            self._notifier.answer_callback_query(cq_id, "Settings closed")

        # ── Language selection ──
        elif data.startswith("lang_"):
            self._cb_language_switch(cq_id, data, chat_id, message_id)

        # ── Mode selection ──
        elif data.startswith("mode_"):
            self._cb_mode_switch(cq_id, data, chat_id, message_id)

        # ── Hybrid AI summary ──
        elif data.startswith("summ_"):
            self._cb_summary(cq_id, data, chat_id, message_id)

        # ── Hybrid show original ──
        elif data.startswith("orig_"):
            self._cb_show_original(cq_id, data, chat_id, message_id)

        # ── Hybrid AI translation ──
        elif data.startswith("trans_"):
            self._cb_translate(cq_id, data, chat_id, message_id)

        # ── Rules inline buttons ──
        elif data == "rules_add":
            with self._rules_lock:
                self._rules_pending[chat_id] = "add"
            self._notifier.answer_callback_query(cq_id, "Send me the rule text")
            self._notifier._api_call("sendMessage", {
                "chat_id": chat_id,
                "text": "📝 Send me the rule text you want to add.",
                "parse_mode": "HTML",
            })
        elif data == "rules_delete":
            with self._rules_lock:
                self._rules_pending[chat_id] = "delete"
            self._notifier.answer_callback_query(cq_id, "Send a rule number to delete")
            self._notifier._api_call("sendMessage", {
                "chat_id": chat_id,
                "text": "🗑 Send me the rule number to delete.",
                "parse_mode": "HTML",
            })
        elif data == "rules_close":
            self._notifier.delete_message(chat_id, message_id)
            self._notifier.answer_callback_query(cq_id, "Rules closed")

        else:
            self._notifier.answer_callback_query(cq_id, "Unknown action")

    # ──────────────────────────────────────────────
    #  Callback implementations
    # ──────────────────────────────────────────────

    def _cb_language_switch(
        self,
        cq_id: str,
        data: str,
        chat_id: str,
        message_id: int,
    ) -> None:
        """Handle language switch callback: lang_zh / lang_en / lang_ja / lang_auto."""
        lang_code = data.replace("lang_", "")

        if lang_code == self.language:
            from core.notifiers.telegram import LANGUAGE_LABELS
            self._notifier.answer_callback_query(
                cq_id, f"Already set to {LANGUAGE_LABELS.get(lang_code, lang_code)}"
            )
            return

        old = self.language
        self.language = lang_code
        self._ai_config.language = lang_code
        logger.info("Language switched: %s → %s", old, lang_code)

        # Persist to config.json
        self._persist_ai_config()

        from core.notifiers.telegram import LANGUAGE_LABELS
        label = LANGUAGE_LABELS.get(lang_code, lang_code)
        self._notifier.answer_callback_query(cq_id, f"✅ Language set to {label}")

        # Return to main settings menu
        self._notifier.edit_settings_main(
            chat_id, message_id, self.mode, self.language,
        )

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

        # Persist
        self._persist_ai_config()

        from core.notifiers.telegram import MODE_LABELS
        label = MODE_LABELS.get(new_mode, new_mode.value)
        self._notifier.answer_callback_query(cq_id, f"✅ Switched to {label}")

        # Return to main settings menu
        self._notifier.edit_settings_main(
            chat_id, message_id, self.mode, self.language,
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

        # Show typing status
        self._notifier.send_chat_action(chat_id)

        runtime_config = self._runtime_ai_config()

        result = analyze_email(
            subject=snapshot.subject,
            sender=snapshot.sender,
            body=snapshot.body_text,
            config=runtime_config,
            rules_block=self.rules_block,
        )

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

        if snapshot.web_link:
            lines.append(f"\n🔗 <a href=\"{snapshot.web_link}\">Open in webmail</a>")

        text = "\n".join(lines)

        self._notifier._api_call(
            "sendMessage",
            {
                "chat_id": chat_id,
                "text": text,
                "parse_mode": "HTML",
                "reply_to_message_id": message_id,
            },
        )

    def _cb_translate(
        self,
        cq_id: str,
        data: str,
        chat_id: str,
        message_id: int,
    ) -> None:
        """
        Handle hybrid mode AI translation callback: trans_{uid}.
        
        Retrieves cached email, runs AI analysis with current language settings,
        and sends translation result as a new message (not editing original).
        
        Falls back gracefully if email cache has expired.
        """
        uid = data.replace("trans_", "")

        self._notifier.answer_callback_query(cq_id, "Generating translation…")

        snapshot = self._get_cached_email(uid)
        if not snapshot:
            logger.warning("No cached email for uid=%s", uid)
            self._notifier.edit_message_text(
                chat_id,
                message_id,
                "⚠️ Email cache expired; cannot generate translation.",
            )
            return

        # Show typing status
        self._notifier.send_chat_action(chat_id)

        runtime_config = self._runtime_ai_config()

        result = analyze_email(
            subject=snapshot.subject,
            sender=snapshot.sender,
            body=snapshot.body_text,
            config=runtime_config,
            rules_block=self.rules_block,
        )

        esc = TelegramNotifier._escape_html

        lines = [
            f"🌐 <b>Translation</b>",
        ]
        
        if result.translation:
            lines.append(f"{esc(result.translation)}")
        else:
            lines.append("⚠️ No translation available.")
        
        if snapshot.web_link:
            lines.append(f"\n🔗 <a href=\"{snapshot.web_link}\">Open in webmail</a>")

        text = "\n".join(lines)

        self._notifier._api_call(
            "sendMessage",
            {
                "chat_id": chat_id,
                "text": text,
                "parse_mode": "HTML",
                "reply_to_message_id": message_id,
            },
        )

    def _cb_show_original(
        self,
        cq_id: str,
        data: str,
        chat_id: str,
        message_id: int,
    ) -> None:
        """Handle hybrid mode show original email callback: orig_{uid}."""
        uid = data.replace("orig_", "")

        self._notifier.answer_callback_query(cq_id, "Loading original email…")

        snapshot = self._get_cached_email(uid)
        if not snapshot:
            logger.warning("No cached email for uid=%s", uid)
            self._notifier.edit_message_text(
                chat_id,
                message_id,
                "⚠️ Email cache expired; cannot show original.",
            )
            return

        esc = TelegramNotifier._escape_html

        lines = [
            f"📧 <b>Original Email</b>",
            f"👤 From: {esc(snapshot.sender)}",
            f"📌 Subject: {esc(snapshot.subject)}",
        ]
        if snapshot.date:
            lines.append(f"🕐 Date: {snapshot.date.strftime('%Y-%m-%d %H:%M:%S')}")
        
        lines.append(f"\n📝 <b>Content:</b>\n{esc(snapshot.body_text)}")
        
        if snapshot.web_link:
            lines.append(f"\n🔗 <a href=\"{snapshot.web_link}\">Open in webmail</a>")

        text = "\n".join(lines)

        self._notifier._api_call(
            "sendMessage",
            {
                "chat_id": chat_id,
                "text": text,
                "parse_mode": "HTML",
                "reply_to_message_id": message_id,
            },
        )

    # ──────────────────────────────────────────────
    #  Helpers
    # ──────────────────────────────────────────────

    def _runtime_ai_config(self) -> AIConfig:
        """Return a snapshot of AI config reflecting current runtime state."""
        # We mutate the shared config object safely — it's fine for single-writer
        self._ai_config.language = self.language
        self._ai_config.enabled = self.ai_enabled
        return self._ai_config
