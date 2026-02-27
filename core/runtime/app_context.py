"""Application context container for composed dependencies."""

from __future__ import annotations

from dataclasses import dataclass

from core.models import AppConfig
from core.runtime.event_bus import EventBus
from service.mailbot.manager import ServiceManager


@dataclass
class AppContext:
    config: AppConfig
    event_bus: EventBus
    service_manager: ServiceManager
