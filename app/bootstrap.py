"""Composition root for service-core architecture."""

from __future__ import annotations

from pathlib import Path

from core.models import AppConfig
from core.runtime.app_context import AppContext
from core.runtime.event_bus import EventBus
from service.mailbot.manager import ServiceManager


def build_app(config_path: Path | None = None) -> AppContext:
    if config_path and config_path.exists():
        config = AppConfig.load(config_path)
    else:
        config = AppConfig()

    bus = EventBus()
    manager = ServiceManager(config)
    return AppContext(config=config, event_bus=bus, service_manager=manager)
