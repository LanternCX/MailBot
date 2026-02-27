"""Structural typing protocols for email adapters."""

from __future__ import annotations

from typing import Protocol

from core.models import EmailSnapshot


class EmailSourceProtocol(Protocol):
    def poll_new_emails(self) -> list[EmailSnapshot]: ...
