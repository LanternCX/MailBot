"""MailBot pipeline event handlers."""

from __future__ import annotations

from core.models import OperationMode
from core.runtime.events import EmailReceived, ForwardRequested, SummaryRequested
from service.mailbot import policies


class MailPipelineService:
    """Translate incoming emails into downstream business events."""

    def __init__(self, publish) -> None:  # noqa: ANN001
        self._publish = publish

    def on_email_received(self, event: EmailReceived, mode: OperationMode) -> None:
        if policies.should_send_preview(mode):
            self._publish(ForwardRequested(email=event.email))
        if policies.should_run_upfront_ai(mode):
            self._publish(SummaryRequested(email=event.email))
