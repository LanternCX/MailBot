"""In-process event bus."""

from __future__ import annotations

import logging
from collections import defaultdict
from collections.abc import Callable
from typing import Any

logger = logging.getLogger("mailbot.runtime.event_bus")

EventHandler = Callable[[Any], None]


class EventBus:
    """Synchronous pub/sub event bus for module decoupling."""

    def __init__(self) -> None:
        self._handlers: dict[type[Any], list[EventHandler]] = defaultdict(list)

    def subscribe(self, event_type: type[Any], handler: EventHandler) -> None:
        self._handlers[event_type].append(handler)

    def publish(self, event: Any) -> None:
        for handler in list(self._handlers.get(type(event), [])):
            try:
                handler(event)
            except Exception:
                logger.exception("Event handler failed for %s", type(event).__name__)
