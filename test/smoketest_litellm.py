"""Smoke-test: verify litellm imports correctly in a PyInstaller bundle.

Usage (run from project root):
    # 1. build the test binary
    .venv/bin/python -m PyInstaller --clean --onefile \
        --additional-hooks-dir hooks \
        --hidden-import litellm --collect-data litellm \
        --name smoketest scripts/smoketest_litellm.py

    # 2. run it
    ./dist/smoketest

Expected output: "OK — litellm <version> imported successfully"
"""

from __future__ import annotations

import sys


def main() -> int:
    frozen = getattr(sys, "frozen", False)
    print(f"frozen={frozen}  _MEIPASS={getattr(sys, '_MEIPASS', 'N/A')}")

    try:
        import litellm

        ver = getattr(litellm, "__version__", "unknown")
        print(f"OK — litellm {ver} imported successfully")
    except FileNotFoundError as exc:
        print(f"FAIL — FileNotFoundError: {exc}")
        return 1
    except ImportError as exc:
        print(f"FAIL — ImportError: {exc}")
        return 1

    # Verify cost map loaded (may be empty dict, that's OK)
    cost = getattr(litellm, "model_cost", None)
    print(f"model_cost type={type(cost).__name__}  entries={len(cost) if cost else 0}")

    # Verify tiktoken encoding works (litellm uses it for token counting)
    try:
        import tiktoken

        enc = tiktoken.get_encoding("cl100k_base")
        tokens = enc.encode("hello world")
        print(f"OK — tiktoken cl100k_base encoding works (tokens={len(tokens)})")
    except Exception as exc:
        print(f"FAIL — tiktoken encoding error: {exc}")
        return 1

    # Verify socksio is importable (needed for SOCKS proxy via httpx)
    try:
        import socksio  # noqa: F401

        print("OK — socksio importable (SOCKS proxy support available)")
    except ImportError:
        print("WARN — socksio not found (SOCKS proxy will not work)")

    # Verify httpx can actually create a SOCKS proxy transport
    try:
        import httpx

        client = httpx.Client(proxy="socks5://127.0.0.1:1080")
        client.close()
        print("OK — httpx SOCKS proxy transport created successfully")
    except ImportError as exc:
        print(f"FAIL — httpx SOCKS proxy transport: {exc}")
        return 1
    except Exception:
        # Connection errors are expected (no real proxy), we only care about ImportError
        print("OK — httpx SOCKS proxy transport creation did not raise ImportError")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
