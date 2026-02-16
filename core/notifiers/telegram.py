"""
core/notifiers/telegram.py
~~~~~~~~~~~~~~~~~~~~~~~~~~
Telegram notifier implementation using Bot API.
"""

from __future__ import annotations

import logging
from typing import Any

import requests

from core.models import EmailSnapshot, TelegramNotifierConfig
from core.notifiers.base import BaseNotifier

logger = logging.getLogger("mailbot.notifier.telegram")


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

    def send(self, snapshot: EmailSnapshot) -> bool:
        """Send notification via Telegram Bot API."""
        message_text = self.format_message(snapshot)
        payload: dict[str, Any] = {
            "chat_id": self._config.chat_id,
            "text": message_text,
            "parse_mode": "HTML",
            "disable_web_page_preview": False,
        }

        # Escape HTML special characters
        payload["text"] = self._escape_html(message_text)

        url = f"{self._api_url}/sendMessage"

        try:
            response = self._session.post(
                url,
                json=payload,
                timeout=self._config.timeout,
            )
            if response.status_code == 200:
                data = response.json()
                if data.get("ok"):
                    logger.info(
                        "Telegram sent: %s -> %s",
                        snapshot.subject,
                        self._config.chat_id,
                    )
                    return True
                else:
                    logger.error("Telegram API error: %s", data.get("description", "Unknown error"))
                    return False
            elif response.status_code == 401:
                logger.error("Telegram auth failed (401), check bot token")
                return False
            elif response.status_code == 429:
                retry_after = response.json().get("parameters", {}).get(
                    "retry_after", 30
                )
                logger.warning("Telegram rate limited, wait %d seconds", retry_after)
                return False
            else:
                logger.error(
                    "Telegram request failed — HTTP %d: %s",
                    response.status_code,
                    response.text[:200],
                )
                return False

        except requests.exceptions.Timeout:
            logger.warning("Telegram request timed out (%d s)", self._config.timeout)
            return False
        except requests.exceptions.ConnectionError:
            logger.warning("Telegram connection failed, check network/API base")
            return False
        except Exception:
            logger.exception("Unexpected error while sending Telegram notification")
            return False

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
        url = f"{self._api_url}/getMe"
        try:
            resp = self._session.get(url, timeout=self._config.timeout)
            if resp.status_code == 200 and resp.json().get("ok"):
                bot_name = resp.json()["result"].get("username", "Unknown")
                logger.info("Telegram Bot connected: @%s", bot_name)
                return True
            logger.error("Telegram Bot token validation failed")
            return False
        except Exception:
            logger.exception("Telegram connectivity test failed")
            return False
