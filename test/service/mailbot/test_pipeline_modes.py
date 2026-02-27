from __future__ import annotations

from datetime import datetime

from core.models import EmailSnapshot, OperationMode
from core.runtime.events import EmailReceived, ForwardRequested, SummaryRequested
from service.mailbot.pipeline import MailPipelineService


class _Recorder:
    def __init__(self) -> None:
        self.events: list[object] = []

    def __call__(self, event: object) -> None:
        self.events.append(event)


def _event() -> EmailReceived:
    return EmailReceived(
        email=EmailSnapshot(
            uid="u",
            account_name="A",
            subject="S",
            sender="a@example.com",
            date=datetime.now(),
            body_text="body",
        )
    )


def test_raw_mode_requests_forward() -> None:
    recorder = _Recorder()
    svc = MailPipelineService(publish=recorder)
    svc.on_email_received(_event(), OperationMode.RAW)
    assert any(isinstance(evt, ForwardRequested) for evt in recorder.events)


def test_agent_mode_requests_summary() -> None:
    recorder = _Recorder()
    svc = MailPipelineService(publish=recorder)
    svc.on_email_received(_event(), OperationMode.AGENT)
    assert any(isinstance(evt, SummaryRequested) for evt in recorder.events)
