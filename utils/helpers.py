"""
utils/helpers.py
~~~~~~~~~~~~~~~~
Shared CLI helpers: banners, tables, formatting.
"""

from __future__ import annotations

import os
import socket

from rich.console import Console
from rich.panel import Panel
from rich.table import Table

from core.models import ProxyConfig

console = Console()

# Preserve the original socket class so we can restore when proxy is disabled
_ORIGINAL_SOCKET = socket.socket

BANNER = r"""
   __  ___      _ __  ____        __
  /  |/  /___ _(_) / / __ )____  / /_
 / /|_/ / __ `/ / / / __  / __ \/ __/
/ /  / / /_/ / / / / /_/ / /_/ / /_
/_/  /_/\__,_/_/_/ /_____/\____/\__/
"""

VERSION = "1.0.0"


def show_banner() -> None:
    """Print ASCII banner with version."""
    console.print(
        Panel(
            f"[bold cyan]{BANNER}[/bold cyan]\n"
            f"  [dim]v{VERSION} — IMAP → Telegram forwarder[/dim]",
            border_style="cyan",
            expand=False,
        )
    )


def show_accounts_table(accounts: list[dict]) -> None:
    """Render accounts as a Rich table."""
    table = Table(title="Configured Accounts", show_lines=True)
    table.add_column("#", style="dim", width=3)
    table.add_column("Name", style="bold")
    table.add_column("Email")
    table.add_column("IMAP Host")
    table.add_column("SSL", justify="center")
    table.add_column("Enabled", justify="center")

    for idx, acc in enumerate(accounts, 1):
        ssl_mark = "✓" if acc.get("use_ssl", True) else "✗"
        ena_mark = "[green]✓[/]" if acc.get("enabled", True) else "[red]✗[/]"
        table.add_row(
            str(idx),
            acc.get("name", ""),
            acc.get("email", ""),
            f"{acc.get('imap_host', '')}:{acc.get('imap_port', 993)}",
            ssl_mark,
            ena_mark,
        )

    console.print(table)


def show_bot_table(notifiers: list[dict]) -> None:
    """Render notifier settings as a Rich table."""
    table = Table(title="Notifier Settings", show_lines=True)
    table.add_column("#", style="dim", width=3)
    table.add_column("Type", style="bold")
    table.add_column("Enabled", justify="center")
    table.add_column("Details")

    for idx, n in enumerate(notifiers, 1):
        ena = "[green]✓[/]" if n.get("enabled", True) else "[red]✗[/]"
        detail = ""
        if n.get("type") == "telegram" and n.get("telegram"):
            tg = n["telegram"]
            token_preview = str(tg.get("bot_token", ""))[:12] + "…"
            detail = f"chat={tg.get('chat_id', '?')}  token={token_preview}"
        table.add_row(str(idx), n.get("type", "?"), ena, detail)

    console.print(table)


def confirm_or_abort(msg: str = "Continue?") -> bool:
    """Quick y/n confirmation via Rich prompt."""
    answer = console.input(f"[yellow]{msg} [y/N]: [/]").strip().lower()
    return answer in ("y", "yes")


def apply_global_proxy(proxy: ProxyConfig | None) -> None:
    """Apply or clear a global proxy for HTTP (requests) and IMAP (socket).

    This sets HTTP(S)_PROXY environment variables and, when enabled, monkey-patches
    the default socket to route IMAP (imaplib) traffic through the proxy using PySocks.
    """
    global _ORIGINAL_SOCKET

    # Helper to restore default socket
    def _restore_socket() -> None:
        if socket.socket is not _ORIGINAL_SOCKET:
            socket.socket = _ORIGINAL_SOCKET

    # Clear env proxies first
    for key in ("http_proxy", "https_proxy", "HTTP_PROXY", "HTTPS_PROXY"):
        os.environ.pop(key, None)

    if not proxy or not proxy.enabled:
        _restore_socket()
        return

    try:
        import socks  # type: ignore
    except Exception:
        console.print("[red]Proxy requested but PySocks is missing. Install with: pip install PySocks[/red]")
        _restore_socket()
        return

    proxy_url = proxy.as_url()
    os.environ["http_proxy"] = proxy_url
    os.environ["https_proxy"] = proxy_url
    os.environ["HTTP_PROXY"] = proxy_url
    os.environ["HTTPS_PROXY"] = proxy_url

    scheme = proxy.scheme.lower()
    if scheme == "socks5":
        socks_type = socks.SOCKS5
    elif scheme == "socks4":
        socks_type = socks.SOCKS4
    else:
        socks_type = socks.HTTP

    socks.set_default_proxy(
        socks_type,
        addr=proxy.host,
        port=proxy.port,
        username=proxy.username,
        password=proxy.password.get_secret_value() if proxy.password else None,
    )

    # Route all future socket connections through the proxy
    socket.socket = socks.socksocket
