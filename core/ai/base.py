"""Abstract AI gateway contracts."""

from __future__ import annotations

from abc import ABC, abstractmethod
from typing import Any


class AIBase(ABC):
    @abstractmethod
    def set_profile(self, profile: dict[str, Any]) -> None: ...

    @abstractmethod
    def chat(
        self,
        messages: list[dict[str, str]],
        profile: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> str: ...

    @abstractmethod
    def chat_json(
        self,
        messages: list[dict[str, str]],
        schema: dict[str, Any] | None = None,
        profile: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> dict[str, Any]: ...
