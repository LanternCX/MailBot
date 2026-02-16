"""
utils/helpers.py
~~~~~~~~~~~~~~~~
Shared CLI helpers: banners, tables, formatting.
"""

from __future__ import annotations

from rich.console import Console
from rich.panel import Panel
from rich.table import Table

console = Console()

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
