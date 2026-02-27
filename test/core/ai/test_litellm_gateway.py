from __future__ import annotations

from core.ai.adapters.litellm import LiteLLMGateway


def test_gateway_profile_setter() -> None:
    gateway = LiteLLMGateway()
    gateway.set_profile({"model": "gpt-4o-mini"})
    assert gateway.default_profile["model"] == "gpt-4o-mini"
