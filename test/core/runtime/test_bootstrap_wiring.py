from __future__ import annotations

from app.bootstrap import build_app


def test_bootstrap_wires_service_manager() -> None:
    app = build_app()
    assert app.service_manager is not None
    assert app.event_bus is not None
