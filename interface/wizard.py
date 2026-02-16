"""
interface/wizard.py
~~~~~~~~~~~~~~~~~~~
Step-by-step config dialogs using questionary.
"""

from __future__ import annotations

import logging

import questionary
from pydantic import SecretStr
from rich.console import Console

from core.models import (
    AccountConfig,
    AppConfig,
    NotifierConfig,
    TelegramNotifierConfig,
)

logger = logging.getLogger("mailbot.wizard")
console = Console()

# ── Provider presets ──

PROVIDERS: dict[str, dict[str, object]] = {
    "Gmail": {"imap_host": "imap.gmail.com", "imap_port": 993, "use_ssl": True},
    "Outlook": {"imap_host": "outlook.office365.com", "imap_port": 993, "use_ssl": True},
    "QQ Mail": {"imap_host": "imap.qq.com", "imap_port": 993, "use_ssl": True},
    "163 Mail": {"imap_host": "imap.163.com", "imap_port": 993, "use_ssl": True},
    "Custom": {},
}


def account_wizard() -> AccountConfig | None:
    """Interactive wizard to add one IMAP account. Return None on cancel."""
    console.print("\n[bold cyan]── Add Email Account ──[/bold cyan]")

    # Step 1: Provider
    provider = questionary.select(
        "Select provider:",
        choices=list(PROVIDERS.keys()),
        qmark="▸",
        pointer="›",
    ).ask()
    if provider is None:
        return None

    preset = PROVIDERS[provider]

    # Step 2: Credentials
    name = questionary.text(
        "Display name:",
        default=provider,
        qmark="▸",
    ).ask()
    if not name:
        return None

    email = questionary.text(
        "Email address:",
        qmark="▸",
        validate=lambda v: True if "@" in v else "Invalid email",
    ).ask()
    if not email:
        return None

    password = questionary.password(
        "App password:",
        qmark="▸",
    ).ask()
    if not password:
        return None

    # Step 3: IMAP host/port (prefilled for known providers)
    if provider == "Custom":
        imap_host = questionary.text(
            "IMAP host:",
            qmark="▸",
        ).ask()
        if not imap_host:
            return None

        port_str = questionary.text(
            "IMAP port:",
            default="993",
            qmark="▸",
        ).ask()
        try:
            imap_port = int(port_str or "993")
        except ValueError:
            imap_port = 993

        use_ssl = questionary.confirm(
            "Use SSL?",
            default=True,
            qmark="▸",
        ).ask()
        if use_ssl is None:
            use_ssl = True
    else:
        imap_host = str(preset["imap_host"])
        imap_port = int(preset.get("imap_port", 993))  # type: ignore[arg-type]
        use_ssl = bool(preset.get("use_ssl", True))

    # Step 4: Folders
    folders_raw = questionary.text(
        "Folders (comma separated):",
        default="INBOX",
        qmark="▸",
    ).ask()
    folders = [f.strip() for f in (folders_raw or "INBOX").split(",") if f.strip()]

    # Step 5: Optional web URL
    web_url = questionary.text(
        "Webmail URL prefix (optional):",
        default="",
        qmark="▸",
    ).ask() or ""

    # Step 6: Verify connection?
    verify = questionary.confirm(
        "Verify IMAP connection now?",
        default=True,
        qmark="▸",
    ).ask()

    acc = AccountConfig(
        name=name,
        email=email,
        password=SecretStr(password),
        imap_host=imap_host,
        imap_port=imap_port,
        use_ssl=use_ssl,
        folders=folders,
        enabled=True,
        web_url=web_url,
    )

    if verify:
        _verify_imap(acc)

    return acc


def bot_wizard(config: AppConfig) -> NotifierConfig | None:
    """Interactive wizard to set Telegram bot token & chat ID."""
    console.print("\n[bold cyan]── Telegram Bot Setup ──[/bold cyan]")

    # Prefill from existing config
    existing_token = ""
    existing_chat = ""
    existing_api = "https://api.telegram.org"
    for n in config.notifiers:
        if n.type == "telegram" and n.telegram:
            existing_token = n.telegram.bot_token.get_secret_value()
            existing_chat = n.telegram.chat_id
            existing_api = n.telegram.api_base
            break

    token = questionary.password(
        "Bot token:",
        default=existing_token,
        qmark="▸",
    ).ask()
    if not token:
        return None

    chat_id = questionary.text(
        "Chat ID:",
        default=existing_chat,
        qmark="▸",
    ).ask()
    if not chat_id:
        return None

    api_base = questionary.text(
        "API base URL:",
        default=existing_api,
        qmark="▸",
    ).ask() or "https://api.telegram.org"

    return NotifierConfig(
        type="telegram",
        enabled=True,
        telegram=TelegramNotifierConfig(
            bot_token=SecretStr(token),
            chat_id=chat_id,
            api_base=api_base,
        ),
    )


# ── Helpers ──


def _verify_imap(acc: AccountConfig) -> None:
    """Try IMAP login and report result."""
    from imap_tools import MailBox, MailBoxUnencrypted, MailboxLoginError

    console.print("[dim]Checking IMAP connection…[/dim]")
    MailBoxCls = MailBox if acc.use_ssl else MailBoxUnencrypted
    try:
        with MailBoxCls(
            host=acc.imap_host,
            port=acc.imap_port,
        ).login(
            username=acc.email,
            password=acc.password.get_secret_value(),
        ):
            console.print("[green]Success: IMAP login OK.[/green]")
    except MailboxLoginError:
        console.print("[red]Error: Authentication failed. Check email/password.[/red]")
    except (OSError, TimeoutError) as exc:
        console.print(f"[red]Error: Connection failed — {exc}[/red]")
    except Exception as exc:
        console.print(f"[red]Error: {exc}[/red]")
