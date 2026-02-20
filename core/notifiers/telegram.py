"""
core/notifiers/telegram.py
~~~~~~~~~~~~~~~~~~~~~~~~~~
Telegram notifier implementation using Bot API.

Supports:
- Basic message sending
- Inline keyboards (multi-level settings dashboard)
- Message editing
- Callback query answers
- Mode-aware email formatting with translation
"""

from __future__ import annotations

import logging
import re
from typing import Any

import requests

from core.models import (
    AIAnalysisResult,
    EmailSnapshot,
    OperationMode,
    TelegramNotifierConfig,
)
from core.notifiers.base import BaseNotifier

logger = logging.getLogger("mailbot.notifier.telegram")

# Priority label mapping
PRIORITY_LABELS = {1: "🔴 Urgent", 2: "🟠 High", 3: "🟡 Medium", 4: "🔵 Low", 5: "⚪ Lowest"}
CATEGORY_ICONS = {
    "verification_code": "🔑",
    "notification": "📢",
    "billing": "💰",
    "promotion": "📣",
    "personal": "✉️",
}

# Language display metadata
LANGUAGE_OPTIONS: list[tuple[str, str, str]] = [
    # (code, flag+label, callback_data)
    ("zh", "🇨🇳 中文", "lang_zh"),
    ("en", "🇺🇸 English", "lang_en"),
    ("ja", "🇯🇵 日本語", "lang_ja"),
    ("auto", "🌐 Auto-detect", "lang_auto"),
]

LANGUAGE_LABELS: dict[str, str] = {code: label for code, label, _ in LANGUAGE_OPTIONS}
MODE_LABELS: dict[OperationMode, str] = {
    OperationMode.RAW: "Raw",
    OperationMode.HYBRID: "Hybrid",
    OperationMode.AGENT: "Agent",
}


class TelegramNotifier(BaseNotifier):
    """Telegram Bot notifier using the sendMessage endpoint."""

    def __init__(self, config: TelegramNotifierConfig) -> None:
        self._config = config
        self._session = requests.Session()
        self._session.headers.update({"Content-Type": "application/json"})

    @property
    def name(self) -> str:
        return "Telegram"

    @property
    def _api_url(self) -> str:
        token = self._config.bot_token.get_secret_value()
        return f"{self._config.api_base}/bot{token}"

    # ──────────────────────────────────────────────
    #  Core send methods
    # ──────────────────────────────────────────────

    def send(self, snapshot: EmailSnapshot) -> bool:
        """Send notification via Telegram Bot API (raw mode, no AI)."""
        message_text = self.format_message(snapshot)
        return self._send_text(message_text)

    def send_with_mode(
        self,
        snapshot: EmailSnapshot,
        mode: OperationMode,
        ai_result: AIAnalysisResult | None = None,
    ) -> bool:
        """
        Send notification with mode-aware formatting.

        Mode A (Raw):    plain forward
        Mode B (Hybrid): short preview + AI button, or direct forward
        Mode C (Agent):  structured AI card
        """
        if mode == OperationMode.RAW:
            return self.send(snapshot)

        elif mode == OperationMode.HYBRID:
            return self._send_hybrid(snapshot)

        elif mode == OperationMode.AGENT:
            if ai_result:
                return self._send_agent_card(snapshot, ai_result)
            # Fallback if AI result missing
            return self.send(snapshot)

        return self.send(snapshot)

    # ──────────────────────────────────────────────
    #  Hybrid mode
    # ──────────────────────────────────────────────

    def _send_hybrid(self, snapshot: EmailSnapshot) -> bool:
        """
        Hybrid mode: check content and decide format.
        - Short / code-like → direct forward
        - Long content → preview + AI summary button
        """
        from core.ai import should_skip_ai

        body = snapshot.body_text

        if should_skip_ai(body):
            # Short or code-like — send directly without AI button
            return self.send(snapshot)

        # Long content: send preview with AI button
        preview = body[:50]
        if len(body) > 50:
            preview += "…"

        lines = [
            "📬 <b>New mail</b>",
            f"📧 Account: {self._escape_html(snapshot.account_name)}",
            f"👤 From: {self._escape_html(snapshot.sender)}",
            f"📌 Subject: {self._escape_html(snapshot.subject)}",
        ]
        if snapshot.date:
            lines.append(f"🕐 Time: {snapshot.date.strftime('%Y-%m-%d %H:%M')}")
        lines.append(f"\n📝 Preview:\n{self._escape_html(preview)}")
        if snapshot.web_link:
            lines.append(f"\n🔗 <a href=\"{snapshot.web_link}\">Open in webmail</a>")

        text = "\n".join(lines)

        # Inline keyboard with AI summary button
        keyboard = {
            "inline_keyboard": [[
                {"text": "✨ AI Summary", "callback_data": f"summ_{snapshot.uid}"},
            ]]
        }

        return self._send_text(text, reply_markup=keyboard, parse_mode="HTML")

    # ──────────────────────────────────────────────
    #  Agent mode
    # ──────────────────────────────────────────────

    def _send_agent_card(
        self,
        snapshot: EmailSnapshot,
        result: AIAnalysisResult,
    ) -> bool:
        """Send structured AI analysis card (with optional translation)."""
        cat_icon = CATEGORY_ICONS.get(result.category, "📧")
        pri_label = PRIORITY_LABELS.get(result.priority, "🟡 Medium")

        lines = [
            f"{cat_icon} <b>{self._escape_html(result.category)}</b>  |  {pri_label}",
            "",
            f"📌 <b>{self._escape_html(snapshot.subject)}</b>",
            f"👤 {self._escape_html(snapshot.sender)}",
        ]
        if snapshot.date:
            lines.append(f"🕐 {snapshot.date.strftime('%Y-%m-%d %H:%M')}")

        lines.append(f"\n💡 <b>AI Summary</b>\n{self._escape_html(result.summary)}")

        if result.extracted_code:
            lines.append(f"\n🔑 <b>Code</b>: <code>{self._escape_html(result.extracted_code)}</code>")

        # Smart translation block (Feat 4)
        if result.translation:
            lines.append(f"\n🌐 <b>Translation</b>\n{self._escape_html(result.translation)}")

        if snapshot.web_link:
            lines.append(f"\n🔗 <a href=\"{snapshot.web_link}\">Open in webmail</a>")

        text = "\n".join(lines)
        return self._send_text(text, parse_mode="HTML")

    # ──────────────────────────────────────────────
    #  /settings dashboard (multi-level inline keyboard)
    # ──────────────────────────────────────────────

    def send_settings_panel(
        self,
        current_mode: OperationMode,
        language: str,
        chat_id: str | None = None,
    ) -> dict | None:
        """
        Send the /settings main-menu inline keyboard panel.

        Returns the API response dict (contains message_id) or None on failure.
        """
        text = self._build_settings_text(current_mode, language)
        keyboard = self._build_settings_main_keyboard(current_mode, language)

        payload: dict[str, Any] = {
            "chat_id": chat_id or self._config.chat_id,
            "text": text,
            "parse_mode": "HTML",
            "reply_markup": keyboard,
        }

        return self._api_call("sendMessage", payload)

    def edit_settings_main(
        self,
        chat_id: str,
        message_id: int,
        current_mode: OperationMode,
        language: str,
    ) -> bool:
        """Edit an existing message to show the settings main menu."""
        text = self._build_settings_text(current_mode, language)
        keyboard = self._build_settings_main_keyboard(current_mode, language)

        payload: dict[str, Any] = {
            "chat_id": chat_id,
            "message_id": message_id,
            "text": text,
            "parse_mode": "HTML",
            "reply_markup": keyboard,
        }
        result = self._api_call("editMessageText", payload)
        return result is not None

    def edit_settings_language_submenu(
        self,
        chat_id: str,
        message_id: int,
        current_language: str,
    ) -> bool:
        """Edit message in-place to show the language selection sub-menu."""
        current_label = LANGUAGE_LABELS.get(current_language, current_language)
        text = (
            "🌐 <b>Language Settings</b>\n\n"
            f"Current: ✅ <b>{current_label}</b>\n\n"
            "Select output language:"
        )

        buttons: list[list[dict]] = []
        row: list[dict] = []
        for code, label, cb_data in LANGUAGE_OPTIONS:
            display = f"✅ {label}" if code == current_language else label
            row.append({"text": display, "callback_data": cb_data})
            if len(row) == 2:
                buttons.append(row)
                row = []
        if row:
            buttons.append(row)

        buttons.append([{"text": "🔙 Back", "callback_data": "settings_back"}])
        keyboard = {"inline_keyboard": buttons}

        payload: dict[str, Any] = {
            "chat_id": chat_id,
            "message_id": message_id,
            "text": text,
            "parse_mode": "HTML",
            "reply_markup": keyboard,
        }
        result = self._api_call("editMessageText", payload)
        return result is not None

    def edit_settings_mode_submenu(
        self,
        chat_id: str,
        message_id: int,
        current_mode: OperationMode,
    ) -> bool:
        """Edit message in-place to show the mode selection sub-menu."""
        current_label = MODE_LABELS.get(current_mode, current_mode.value)
        text = (
            "⚙️ <b>Operation Mode</b>\n\n"
            f"Current: ✅ <b>{current_label}</b>\n\n"
            "Select mode:"
        )

        buttons: list[list[dict]] = []
        for m, label in MODE_LABELS.items():
            display = f"✅ {label}" if m == current_mode else label
            buttons.append([{"text": display, "callback_data": f"mode_{m.value}"}])

        buttons.append([{"text": "🔙 Back", "callback_data": "settings_back"}])
        keyboard = {"inline_keyboard": buttons}

        payload: dict[str, Any] = {
            "chat_id": chat_id,
            "message_id": message_id,
            "text": text,
            "parse_mode": "HTML",
            "reply_markup": keyboard,
        }
        result = self._api_call("editMessageText", payload)
        return result is not None

    def delete_message(self, chat_id: str, message_id: int) -> bool:
        """Delete a message by ID."""
        payload = {"chat_id": chat_id, "message_id": message_id}
        return self._api_call("deleteMessage", payload) is not None

    # ── Settings panel builders ──

    @staticmethod
    def _build_settings_text(
        mode: OperationMode,
        language: str,
    ) -> str:
        mode_label = MODE_LABELS.get(mode, mode.value)
        lang_label = LANGUAGE_LABELS.get(language, language)
        return (
            "⚙️ <b>MailBot Settings</b>\n\n"
            f"🌐 Language: <b>{lang_label}</b>\n"
            f"📋 Mode: <b>{mode_label}</b>\n"
            "Use the CLI or config.json to change AI on/off."
        )

    @staticmethod
    def _build_settings_main_keyboard(
        mode: OperationMode,
        language: str,
    ) -> dict:
        return {
            "inline_keyboard": [
                [{"text": "🌐 Language >", "callback_data": "settings_lang"}],
                [{"text": f"⚙️ Mode: {MODE_LABELS.get(mode, mode.value)} >", "callback_data": "settings_mode"}],
                [{"text": "❌ Close", "callback_data": "settings_close"}],
            ]
        }

    # ──────────────────────────────────────────────
    #  /ai reply analysis
    # ──────────────────────────────────────────────

    def send_ai_result(
        self,
        chat_id: str,
        reply_to_message_id: int,
        result: AIAnalysisResult,
    ) -> bool:
        """Send AI analysis result as a reply to a message."""
        cat_icon = CATEGORY_ICONS.get(result.category, "📧")
        pri_label = PRIORITY_LABELS.get(result.priority, "🟡 Medium")

        lines = [
            f"🤖 <b>AI Analysis</b>",
            "",
            f"{cat_icon} Category: <b>{self._escape_html(result.category)}</b>",
            f"📊 Priority: {pri_label}",
            f"\n💡 Summary:\n{self._escape_html(result.summary)}",
        ]
        if result.extracted_code:
            lines.append(f"\n🔑 Code: <code>{self._escape_html(result.extracted_code)}</code>")

        text = "\n".join(lines)

        payload: dict[str, Any] = {
            "chat_id": chat_id,
            "text": text,
            "parse_mode": "HTML",
            "reply_to_message_id": reply_to_message_id,
        }

        return self._api_call("sendMessage", payload) is not None

    # ──────────────────────────────────────────────
    #  Callback query handling
    # ──────────────────────────────────────────────

    def answer_callback_query(self, callback_query_id: str, text: str = "") -> bool:
        """Send a toast notification for a callback query."""
        payload = {"callback_query_id": callback_query_id, "text": text}
        return self._api_call("answerCallbackQuery", payload) is not None

    def edit_message_text(
        self,
        chat_id: str,
        message_id: int,
        text: str,
        parse_mode: str = "HTML",
        reply_markup: dict | None = None,
    ) -> bool:
        """Edit the text of an existing message."""
        payload: dict[str, Any] = {
            "chat_id": chat_id,
            "message_id": message_id,
            "text": text,
            "parse_mode": parse_mode,
        }
        if reply_markup:
            payload["reply_markup"] = reply_markup
        return self._api_call("editMessageText", payload) is not None

    # ──────────────────────────────────────────────
    #  Bot update polling
    # ──────────────────────────────────────────────

    def get_updates(self, offset: int | None = None, timeout: int = 30) -> list[dict]:
        """Long-poll for new updates from the Bot API."""
        payload: dict[str, Any] = {
            "timeout": timeout,
            "allowed_updates": ["message", "callback_query"],
        }
        if offset is not None:
            payload["offset"] = offset

        result = self._api_call("getUpdates", payload)
        if result and isinstance(result.get("result"), list):
            return result["result"]
        return []

    def set_bot_commands(self, commands: list[dict[str, str]]) -> bool:
        """Register bot commands via setMyCommands."""
        payload = {"commands": commands}
        return self._api_call("setMyCommands", payload) is not None

    def send_chat_action(self, chat_id: str, action: str = "typing") -> bool:
        """
        Send a chat action (e.g., 'typing') to show user that bot is processing.
        
        Args:
            chat_id: Target chat ID
            action: Action type ('typing', 'upload_photo', 'record_video', etc.)
        
        Returns:
            True if successful, False otherwise.
        """
        payload = {"chat_id": chat_id, "action": action}
        return self._api_call("sendChatAction", payload) is not None

    # ──────────────────────────────────────────────
    #  Low-level API helpers
    # ──────────────────────────────────────────────

    def _send_text(
        self,
        text: str,
        reply_markup: dict | None = None,
        parse_mode: str = "HTML",
    ) -> bool:
        """Send a text message, return True on success."""
        payload: dict[str, Any] = {
            "chat_id": self._config.chat_id,
            "text": text,
            "parse_mode": parse_mode,
            "disable_web_page_preview": False,
        }
        if reply_markup:
            payload["reply_markup"] = reply_markup

        # Only escape HTML ourselves if no inline markup is used
        if parse_mode == "HTML" and not reply_markup and "<" not in text[:20]:
            payload["text"] = self._escape_html(text)

        result = self._api_call("sendMessage", payload)
        return result is not None

    def _api_call(self, method: str, payload: dict[str, Any]) -> dict | None:
        """
        Make a Telegram Bot API call.

        Returns the full response dict on success, None on failure.
        """
        url = f"{self._api_url}/{method}"

        try:
            response = self._session.post(
                url,
                json=payload,
                timeout=self._config.timeout,
            )
            if response.status_code == 200:
                data = response.json()
                if data.get("ok"):
                    return data
                logger.error("Telegram API error [%s]: %s", method, data.get("description", "Unknown"))
                return None
            elif response.status_code == 401:
                logger.error("Telegram auth failed (401), check bot token")
                return None
            elif response.status_code == 429:
                retry_after = response.json().get("parameters", {}).get("retry_after", 30)
                logger.warning("Telegram rate limited, wait %d seconds", retry_after)
                return None
            else:
                logger.error(
                    "Telegram [%s] HTTP %d: %s",
                    method,
                    response.status_code,
                    response.text[:200],
                )
                return None

        except requests.exceptions.Timeout:
            logger.warning("Telegram [%s] timed out (%d s)", method, self._config.timeout)
            return None
        except requests.exceptions.ConnectionError:
            logger.warning("Telegram [%s] connection failed", method)
            return None
        except Exception:
            logger.exception("Unexpected error in Telegram [%s]", method)
            return None

    @staticmethod
    def _escape_html(text: str) -> str:
        """Escape HTML special chars for Telegram parse_mode=HTML."""
        return (
            text.replace("&", "&amp;")
            .replace("<", "&lt;")
            .replace(">", "&gt;")
        )

    def test_connection(self) -> bool:
        """Test Bot Token connectivity."""
        result = self._api_call("getMe", {})
        if result:
            bot_name = result.get("result", {}).get("username", "Unknown")
            logger.info("Telegram Bot connected: @%s", bot_name)
            return True
        return False
