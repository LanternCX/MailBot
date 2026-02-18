"""
Proxy smoke test for LiteLLM completion calls going through an HTTP/S proxy.

Required env vars:
- MAILBOT_LITELLM_API_KEY  -> provider API key
- MAILBOT_LITELLM_PROXY    -> proxy URL, e.g. http://user:pass@127.0.0.1:8080
Optional:
- MAILBOT_LITELLM_BASE_URL -> override API base (for self-hosted gateways)
- MAILBOT_LITELLM_MODEL    -> model name (default: gpt-4o-mini)

Run with:
    python -m unittest test.test_litellm_proxy
"""

from __future__ import annotations

import os
import unittest
from contextlib import contextmanager
from pathlib import Path
from typing import Iterator

import litellm

from core.models import AppConfig


@contextmanager
def _temp_env(env: dict[str, str]) -> Iterator[None]:
    """Temporarily apply env vars and restore after the test."""
    original = {key: os.environ.get(key) for key in env}
    os.environ.update(env)
    try:
        yield
    finally:
        for key, value in original.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value


class LiteLLMProxyTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        config_path = Path(__file__).resolve().parent.parent / "config.json"
        if not config_path.exists():
            raise unittest.SkipTest("config.json not found")

        app_config = AppConfig.load(config_path)
        ai_cfg = app_config.ai
        proxy_cfg = app_config.proxy

        if not ai_cfg.enabled or not ai_cfg.api_key:
            raise unittest.SkipTest("AI not enabled or api_key missing in config.json")
        if not proxy_cfg or not proxy_cfg.enabled:
            raise unittest.SkipTest("Proxy not enabled in config.json")

        cls.api_key = ai_cfg.api_key.get_secret_value()
        cls.provider = ai_cfg.provider
        cls.model = ai_cfg.model
        cls.base_url = ai_cfg.base_url
        cls.proxy_url = proxy_cfg.as_url()

    def setUp(self) -> None:
        if not hasattr(self, "api_key"):
            self.skipTest("Config load skipped")

    def test_completion_through_proxy(self) -> None:
        params: dict[str, object] = {
            "api_key": self.api_key,
            "model": f"{self.provider}/{self.model}" if "/" not in self.model else self.model,
            "max_tokens": 32,
            "timeout": 15,
            "temperature": 0,
        }
        if self.base_url:
            params["api_base"] = self.base_url

        print(self.proxy_url)
        litellm.drop_params = True  # ignore provider-specific extras
        litellm.aiohttp_trust_env = True  # honor HTTP_PROXY / HTTPS_PROXY as per docs

        with _temp_env(
            {
                "HTTP_PROXY": self.proxy_url, 
                "HTTPS_PROXY": self.proxy_url, 
                "DEEPSEEK_API_KEY": self.api_key,  
            }
        ):
            response = litellm.completion(
                model=params["model"], 
                messages=[
                    {"role": "user", "content": "hello from litellm"}
                ],
            )
            content = response.choices[0].message.content  # type: ignore[union-attr]
            print("LLM response:", content)
            self.assertIsNotNone(content)


if __name__ == "__main__":
    unittest.main()
