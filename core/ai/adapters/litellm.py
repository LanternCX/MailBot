"""LiteLLM-backed gateway implementation."""

from __future__ import annotations

import json
import logging
from typing import Any

from core.ai.base import AIBase

logger = logging.getLogger("mailbot.ai.gateway")


class LiteLLMGateway(AIBase):
    def __init__(self, default_profile: dict[str, Any] | None = None) -> None:
        self.default_profile: dict[str, Any] = (
            default_profile.copy() if default_profile else {}
        )

    def set_profile(self, profile: dict[str, Any]) -> None:
        self.default_profile = profile.copy()

    def _completion_params(
        self, profile: dict[str, Any] | None = None, **kwargs: Any
    ) -> dict[str, Any]:
        params: dict[str, Any] = {}
        params.update(self.default_profile)
        if profile:
            params.update(profile)
        params.update(kwargs)
        return params

    def chat(
        self,
        messages: list[dict[str, str]],
        profile: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> str:
        try:
            import litellm
        except ImportError as exc:
            raise RuntimeError("litellm is not installed") from exc

        params = self._completion_params(profile=profile, **kwargs)
        response = litellm.completion(messages=messages, **params)
        content = response.choices[0].message.content  # type: ignore[union-attr]
        return str(content) if content else ""

    def chat_json(
        self,
        messages: list[dict[str, str]],
        schema: dict[str, Any] | None = None,
        profile: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> dict[str, Any]:
        params = self._completion_params(profile=profile, **kwargs)
        if schema:
            params["response_format"] = {
                "type": "json_schema",
                "json_schema": {"name": "response", "schema": schema},
            }
        else:
            params["response_format"] = {"type": "json_object"}
        text = self.chat(messages=messages, **params)
        if not text:
            return {}
        try:
            return json.loads(text)
        except json.JSONDecodeError:
            logger.warning("LiteLLM JSON decode failed")
            return {}
