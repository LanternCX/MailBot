"""Email source abstract base class."""

from __future__ import annotations

from abc import ABC, abstractmethod

from core.models import EmailSnapshot


class EmailSourceBase(ABC):
    @abstractmethod
    def poll_new_emails(self) -> list[EmailSnapshot]: ...
