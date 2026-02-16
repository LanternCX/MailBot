"""
core/notifiers/base.py
~~~~~~~~~~~~~~~~~~~~~~
Abstract base class for notifier adapters.

All concrete notifiers (Telegram/Discord/Slack/etc.) must implement ``send``.
Uses the Adapter Pattern to decouple business logic from providers.
"""

from __future__ import annotations

import logging
from abc import ABC, abstractmethod

from core.models import EmailSnapshot

logger = logging.getLogger("mailbot.notifier")


class BaseNotifier(ABC):
    """Abstract notifier interface."""

    @property
    @abstractmethod
    def name(self) -> str:
        """Notifier name for logs and UI."""
        ...

    @abstractmethod
    def send(self, snapshot: EmailSnapshot) -> bool:
        """Send a notification for the snapshot. Return True on success."""
        ...

    def format_message(self, snapshot: EmailSnapshot) -> str:
        """Default message template (override in subclasses if needed)."""
        lines = [
            "📬 New mail",
            f"",
            f"📧 Account: {snapshot.account_name}",
            f"👤 From: {snapshot.sender}",
            f"📌 Subject: {snapshot.subject}",
        ]
        if snapshot.date:
            lines.append(f"🕐 Time: {snapshot.date.strftime('%Y-%m-%d %H:%M')}")
        if snapshot.body_text:
            preview = snapshot.body_text[:500]
            if len(snapshot.body_text) > 500:
                preview += "..."
            lines.append(f"")
            lines.append(f"📝 Preview:\n{preview}")
        if snapshot.web_link:
            lines.append(f"")
            lines.append(f"🔗 Open in webmail: {snapshot.web_link}")
        return "\n".join(lines)

    def __repr__(self) -> str:
        return f"<{self.__class__.__name__} name={self.name!r}>"
