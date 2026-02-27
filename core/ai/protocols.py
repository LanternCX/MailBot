"""Typing protocols for AI gateways."""

from __future__ import annotations

from typing import Any, Protocol


class AIGatewayProtocol(Protocol):
    default_profile: dict[str, Any]

    def set_profile(self, profile: dict[str, Any]) -> None: ...

    def chat(
        self,
        messages: list[dict[str, str]],
        profile: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> str: ...

    def chat_json(
        self,
        messages: list[dict[str, str]],
        schema: dict[str, Any] | None = None,
        profile: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> dict[str, Any]: ...
