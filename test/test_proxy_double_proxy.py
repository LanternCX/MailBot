"""
Test that litellm completion works through proxy when socket is monkey-patched.

This reproduces the double-proxying bug: apply_global_proxy() patches both
env vars AND socket.socket, causing httpx to double-proxy and fail with
SSLEOFError.  The fix in ai.py uses _bypass_socket_proxy() to temporarily
restore the original socket during litellm calls.

Run with:
    python -m pytest test/test_proxy_double_proxy.py -v
"""

from __future__ import annotations

import os
import socket
import unittest
from pathlib import Path
from unittest.mock import MagicMock, patch

from core.ai import _bypass_socket_proxy
from utils.helpers import _ORIGINAL_SOCKET


class BypassSocketProxyTest(unittest.TestCase):
    """Verify _bypass_socket_proxy restores and re-patches socket correctly."""

    def test_restores_original_socket_inside_context(self) -> None:
        """Inside the context manager, socket.socket should be the original."""
        fake_patched = MagicMock(name="socks.socksocket")
        saved = socket.socket

        # Simulate monkey-patched socket
        socket.socket = fake_patched
        try:
            self.assertIs(socket.socket, fake_patched)

            with _bypass_socket_proxy():
                # Inside: should be the original (un-patched) socket
                self.assertIs(socket.socket, _ORIGINAL_SOCKET)

            # After: should be re-patched
            self.assertIs(socket.socket, fake_patched)
        finally:
            socket.socket = saved

    def test_restores_patched_socket_on_exception(self) -> None:
        """Even if the body raises, the patched socket should be restored."""
        fake_patched = MagicMock(name="socks.socksocket")
        saved = socket.socket

        socket.socket = fake_patched
        try:
            with self.assertRaises(RuntimeError):
                with _bypass_socket_proxy():
                    self.assertIs(socket.socket, _ORIGINAL_SOCKET)
                    raise RuntimeError("boom")

            # Patched socket should be restored despite exception
            self.assertIs(socket.socket, fake_patched)
        finally:
            socket.socket = saved

    def test_noop_when_socket_not_patched(self) -> None:
        """When socket is not patched, context manager is a harmless no-op."""
        saved = socket.socket
        # Ensure socket is original
        socket.socket = _ORIGINAL_SOCKET
        try:
            with _bypass_socket_proxy():
                self.assertIs(socket.socket, _ORIGINAL_SOCKET)
            self.assertIs(socket.socket, _ORIGINAL_SOCKET)
        finally:
            socket.socket = saved


class LiveProxyCompletionTest(unittest.TestCase):
    """Live test: litellm completion through proxy with socket patched.

    Skipped when config.json is absent or proxy/AI is disabled.
    """

    @classmethod
    def setUpClass(cls) -> None:
        from core.models import AppConfig

        config_path = Path(__file__).resolve().parent.parent / "config.json"
        if not config_path.exists():
            raise unittest.SkipTest("config.json not found")

        app_config = AppConfig.load(config_path)
        ai_cfg = app_config.ai
        proxy_cfg = app_config.proxy

        if not ai_cfg.enabled or not ai_cfg.api_key:
            raise unittest.SkipTest("AI not enabled or api_key missing")
        if not proxy_cfg or not proxy_cfg.enabled:
            raise unittest.SkipTest("Proxy not enabled")

        cls.app_config = app_config
        cls.ai_cfg = ai_cfg
        cls.proxy_cfg = proxy_cfg

    def test_completion_with_global_proxy_applied(self) -> None:
        """Simulate real app conditions: apply_global_proxy + analyze_email."""
        from utils.helpers import apply_global_proxy
        from core.ai import analyze_email

        # Apply global proxy exactly as main.py does (env vars + socket patch)
        apply_global_proxy(self.proxy_cfg)

        try:
            # Verify socket IS patched (reproducing the bug precondition)
            self.assertIsNot(
                socket.socket, _ORIGINAL_SOCKET,
                "Socket should be monkey-patched after apply_global_proxy",
            )

            # This should succeed (not raise SSLEOFError) thanks to the fix
            result = analyze_email(
                subject="Test Proxy",
                sender="test@example.com",
                body="This is a proxy integration test. Please respond.",
                config=self.ai_cfg,
            )

            self.assertIsNotNone(result)
            self.assertIn(result.category, [
                "verification_code", "notification", "billing",
                "promotion", "personal",
            ])
            print(f"AI result: category={result.category} priority={result.priority}")
        finally:
            # Clean up: restore original socket
            socket.socket = _ORIGINAL_SOCKET
            for key in ("http_proxy", "https_proxy", "HTTP_PROXY", "HTTPS_PROXY"):
                os.environ.pop(key, None)


if __name__ == "__main__":
    unittest.main()
