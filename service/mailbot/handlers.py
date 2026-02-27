"""MailBot event handlers for AI and chat actions."""

from __future__ import annotations

from core.ai import analyze_email
from core.models import AIConfig
from core.runtime.events import SummaryReady, SummaryRequested


class SummaryHandler:
    """Generate summary when summary request events arrive."""

    def __init__(self, config: AIConfig, publish) -> None:  # noqa: ANN001
        self._config = config
        self._publish = publish

    def handle(self, event: SummaryRequested) -> None:
        result = analyze_email(
            subject=event.email.subject,
            sender=event.email.sender,
            body=event.email.body_text,
            config=self._config,
        )
        self._publish(SummaryReady(email=event.email, result=result))
