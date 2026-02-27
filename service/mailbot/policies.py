"""MailBot business policy helpers."""

from __future__ import annotations

from core.models import OperationMode


def should_run_upfront_ai(mode: OperationMode) -> bool:
    return mode == OperationMode.AGENT


def should_send_preview(mode: OperationMode) -> bool:
    return mode in {OperationMode.RAW, OperationMode.HYBRID}
