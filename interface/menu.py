"""
interface/menu.py
~~~~~~~~~~~~~~~~~
Main menu loop and service runner (questionary + Rich).
"""

from __future__ import annotations

import logging
import signal
import time
from pathlib import Path

import questionary
from rich.console import Console

from core.manager import ServiceManager
from core.models import AppConfig
from interface.wizard import account_wizard, bot_wizard
from utils.helpers import (
    show_accounts_table,
    show_banner,
    show_bot_table,
)
from utils.logger import setup_logging

logger = logging.getLogger("mailbot.menu")
console = Console()

# ── Menu choices (constants) ──

CHOICE_START = "Start Service"
CHOICE_CONFIG = "Config Wizard"
CHOICE_BOT = "Bot Settings"
CHOICE_TEST = "Test Connection"
CHOICE_SYSTEM = "System Settings"
CHOICE_EXIT = "Exit"

MENU_CHOICES = [
    CHOICE_START,
    CHOICE_CONFIG,
    CHOICE_BOT,
    CHOICE_SYSTEM,
    CHOICE_TEST,
    CHOICE_EXIT,
]


def main_menu(config_path: Path) -> None:
    """Run the interactive main-menu loop."""
    config = _load_or_default(config_path)
    show_banner()

    while True:
        console.print()
        choice = questionary.select(
            "Main Menu:",
            choices=MENU_CHOICES,
            qmark="▸",
            pointer="›",
        ).ask()

        if choice is None or choice == CHOICE_EXIT:
            console.print("[dim]Bye.[/dim]")
            break
        elif choice == CHOICE_START:
            _run_service(config)
        elif choice == CHOICE_CONFIG:
            config = _config_wizard(config, config_path)
        elif choice == CHOICE_BOT:
            config = _bot_settings(config, config_path)
        elif choice == CHOICE_SYSTEM:
            config = _system_settings(config, config_path)
        elif choice == CHOICE_TEST:
            _test_connection(config)


# ── Handlers ──


def _load_or_default(path: Path) -> AppConfig:
    """Load config or return defaults."""
    if path.exists():
        try:
            return AppConfig.load(path)
        except Exception as exc:
            console.print(f"[red]Error: Failed to parse config — {exc}[/red]")
    return AppConfig()


def _run_service(config: AppConfig) -> None:
    """Start the service in the foreground with live logs."""
    setup_logging(level=config.log_level)

    if not config.accounts:
        console.print("[yellow]Warning: No accounts configured. Run Config Wizard first.[/yellow]")
        return
    if not config.notifiers:
        console.print("[yellow]Warning: No notifiers configured. Run Bot Settings first.[/yellow]")
        return

    manager = ServiceManager(config)
    manager.start()
    console.print("[green]Service started — Ctrl+C to stop[/green]\n")

    # Graceful shutdown on SIGINT
    stop_requested = False

    def _handle_sigint(sig, frame):  # noqa: ANN001
        nonlocal stop_requested
        stop_requested = True

    prev_handler = signal.signal(signal.SIGINT, _handle_sigint)

    try:
        while not stop_requested:
            time.sleep(0.5)
    finally:
        signal.signal(signal.SIGINT, prev_handler)
        console.print("\n[yellow]Stopping service…[/yellow]")
        manager.stop()
        console.print("[green]Service stopped.[/green]")


def _config_wizard(config: AppConfig, config_path: Path) -> AppConfig:
    """Run the account configuration wizard."""
    # Show existing accounts
    if config.accounts:
        raw = [a.model_dump() for a in config.accounts]
        # Mask passwords
        for a in raw:
            a["password"] = "***"
        show_accounts_table(raw)

    action = questionary.select(
        "Account action:",
        choices=["Add Account", "Remove Account", "Back"],
        qmark="▸",
        pointer="›",
    ).ask()

    if action == "Add Account":
        new_acc = account_wizard()
        if new_acc:
            config.accounts.append(new_acc)
            config.save(config_path)
            console.print("[green]Account added and saved.[/green]")

    elif action == "Remove Account":
        if not config.accounts:
            console.print("[dim]No accounts to remove.[/dim]")
        else:
            names = [f"{a.name} ({a.email})" for a in config.accounts]
            to_remove = questionary.select(
                "Remove which account?",
                choices=names + ["Cancel"],
                qmark="▸",
                pointer="›",
            ).ask()
            if to_remove and to_remove != "Cancel":
                idx = names.index(to_remove)
                removed = config.accounts.pop(idx)
                config.save(config_path)
                console.print(f"[green]Removed: {removed.name}[/green]")

    return config


def _system_settings(config: AppConfig, config_path: Path) -> AppConfig:
    """Configure poll interval, retries, and log level."""
    console.print(
        f"[dim]Current:[/dim] interval={config.poll_interval}s  retries={config.max_retries}  log={config.log_level}"
    )

    poll = questionary.text(
        "Polling interval (seconds, >=10):",
        default=str(config.poll_interval),
        validate=lambda v: v.isdigit() and int(v) >= 10 or "Enter integer >=10",
        qmark="▸",
    ).ask()

    retries = questionary.text(
        "Max retries (>=1):",
        default=str(config.max_retries),
        validate=lambda v: v.isdigit() and int(v) >= 1 or "Enter integer >=1",
        qmark="▸",
    ).ask()

    log_level = questionary.select(
        "Log level:",
        choices=["DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"],
        default=config.log_level.upper(),
        qmark="▸",
        pointer="›",
    ).ask()

    if poll and retries and log_level:
        config.poll_interval = int(poll)
        config.max_retries = int(retries)
        config.log_level = log_level.upper()
        config.save(config_path)
        setup_logging(level=config.log_level)
        console.print("[green]System settings saved.[/green]")
    else:
        console.print("[dim]No changes applied.[/dim]")

    return config


def _bot_settings(config: AppConfig, config_path: Path) -> AppConfig:
    """Configure Telegram bot token & chat ID."""
    if config.notifiers:
        raw = []
        for n in config.notifiers:
            d = n.model_dump()
            if d.get("telegram") and d["telegram"].get("bot_token"):
                d["telegram"]["bot_token"] = str(d["telegram"]["bot_token"])[:12] + "…"
            raw.append(d)
        show_bot_table(raw)

    action = questionary.select(
        "Bot action:",
        choices=["Set Telegram Bot", "Remove Bot", "Back"],
        qmark="▸",
        pointer="›",
    ).ask()

    if action == "Set Telegram Bot":
        nc = bot_wizard(config)
        if nc:
            # Replace existing telegram notifier or append
            config.notifiers = [
                n for n in config.notifiers if n.type != "telegram"
            ]
            config.notifiers.append(nc)
            config.save(config_path)
            console.print("[green]Telegram bot saved.[/green]")

    elif action == "Remove Bot":
        if not config.notifiers:
            console.print("[dim]No bots configured.[/dim]")
        else:
            config.notifiers.clear()
            config.save(config_path)
            console.print("[green]All notifiers removed.[/green]")

    return config


def _test_connection(config: AppConfig) -> None:
    """Send a test message via Telegram."""
    from core.notifiers.telegram import TelegramNotifier

    tg_configs = [n for n in config.notifiers if n.type == "telegram" and n.telegram]
    if not tg_configs:
        console.print("[yellow]No Telegram bot configured. Run Bot Settings first.[/yellow]")
        return

    tg_cfg = tg_configs[0].telegram
    assert tg_cfg is not None
    notifier = TelegramNotifier(tg_cfg)

    console.print("[dim]Testing Telegram connection…[/dim]")
    if notifier.test_connection():
        console.print("[green]Success: Bot is reachable.[/green]")

        # Offer to send a test message
        send_test = questionary.confirm(
            "Send test message?",
            default=True,
            qmark="▸",
        ).ask()

        if send_test:
            from core.models import EmailSnapshot
            from datetime import datetime

            snap = EmailSnapshot(
                uid="test-0",
                account_name="Test",
                subject="MailBot Test Message",
                sender="mailbot@local",
                date=datetime.now(),
                body_text="If you see this, the bot is working correctly.",
            )
            if notifier.send(snap):
                console.print("[green]Test message sent.[/green]")
            else:
                console.print("[red]Error: Failed to send test message.[/red]")
    else:
        console.print("[red]Error: Cannot reach Telegram API. Check token & network.[/red]")
