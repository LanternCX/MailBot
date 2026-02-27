from __future__ import annotations

from datetime import datetime

from core.models import EmailSnapshot
from core.runtime.event_bus import EventBus
from core.runtime.events import EmailReceived


def test_event_bus_dispatches_to_subscribers() -> None:
    bus = EventBus()
    seen: list[str] = []

    def _handler(event: EmailReceived) -> None:
        seen.append(event.email.uid)

    snapshot = EmailSnapshot(
        uid="1",
        account_name="A",
        subject="S",
        sender="a@example.com",
        date=datetime.now(),
        body_text="body",
    )
    bus.subscribe(EmailReceived, _handler)
    bus.publish(EmailReceived(email=snapshot))

    assert seen == ["1"]
