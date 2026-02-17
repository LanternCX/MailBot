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
    AIConfig,
    AppConfig,
    NotifierConfig,
    OperationMode,
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


# ── AI Configuration ──

ProviderOption = dict[str, object]

AI_PROVIDER_GROUPS: dict[str, list[ProviderOption]] = {
    "OpenAI & Compatible": [
        {"name": "OpenAI", "provider": "openai", "model": "gpt-4o-mini", "requires_api_key": True},
        {"name": "OpenRouter", "provider": "openrouter", "model": "openrouter/auto", "base_url": "https://openrouter.ai/api/v1"},
        {"name": "Together AI", "provider": "together_ai", "model": "meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo", "base_url": "https://api.together.xyz/v1"},
        {"name": "Fireworks AI", "provider": "fireworks_ai", "model": "accounts/fireworks/models/llama-v3p1-70b-instruct", "base_url": "https://api.fireworks.ai/inference/v1"},
    ],
    "Frontier Models": [
        {"name": "Anthropic", "provider": "anthropic", "model": "claude-3-5-sonnet-latest", "requires_api_key": True},
        {"name": "Google Gemini", "provider": "gemini", "model": "gemini-1.5-flash", "requires_api_key": True},
        {"name": "Mistral", "provider": "mistral", "model": "mistral-large-latest", "requires_api_key": True},
        {"name": "Groq", "provider": "groq", "model": "llama-3.1-70b-versatile", "requires_api_key": True},
    ],
    "China-Friendly": [
        {"name": "DeepSeek", "provider": "deepseek", "model": "deepseek-chat", "requires_api_key": True},
        {"name": "Qwen (Ali Tongyi)", "provider": "qwen", "model": "qwen2-72b-instruct", "requires_api_key": True},
        {"name": "Moonshot", "provider": "moonshot", "model": "moonshot-v1-32k", "requires_api_key": True},
        {"name": "MiniMax", "provider": "minimax", "model": "abab6.5s-chat", "requires_api_key": True},
    ],
    "Research / Web": [
        {"name": "Perplexity", "provider": "perplexity", "model": "pplx-70b-online", "requires_api_key": True},
        {"name": "Cohere", "provider": "cohere", "model": "command-r-plus", "requires_api_key": True},
    ],
    "Local & Custom": [
        {"name": "Ollama (local)", "provider": "ollama", "model": "llama3", "requires_api_key": False, "base_url": "http://localhost:11434", "allow_base_url": True},
        {"name": "OpenAI-Compatible (custom)", "provider": "custom", "model": "gpt-4o-mini", "requires_api_key": True, "allow_base_url": True},
    ],
}


def _find_provider_option(provider: str) -> tuple[str | None, ProviderOption | None]:
    """Locate the provider option by provider id to pre-select defaults."""
    for group_name, options in AI_PROVIDER_GROUPS.items():
        for opt in options:
            if opt.get("provider") == provider:
                return group_name, opt
    return None, None


def ai_wizard(config: AppConfig) -> AIConfig | None:
    """Interactive wizard to configure AI analysis settings."""
    console.print("\n[bold cyan]── AI Configuration ──[/bold cyan]")

    existing = config.ai

    # Step 1: Enable AI?
    enabled = questionary.confirm(
        "Enable AI analysis?",
        default=existing.enabled,
        qmark="▸",
    ).ask()
    if enabled is None:
        return None

    if not enabled:
        return AIConfig(enabled=False)

    # Step 2: Provider group → provider
    existing_group, existing_opt = _find_provider_option(existing.provider)
    group_names = list(AI_PROVIDER_GROUPS.keys())
    provider_group = questionary.select(
        "Select provider group:",
        choices=group_names,
        default=existing_group or group_names[0],
        qmark="▸",
        pointer="›",
    ).ask()
    if provider_group is None:
        return None

    group_options = AI_PROVIDER_GROUPS[provider_group]
    default_provider_id = str(
        existing_opt.get("provider") if existing_opt and existing_group == provider_group else group_options[0]["provider"]
    )
    provider_id = questionary.select(
        "Select AI platform:",
        choices=[
            questionary.Choice(
                title=f"{opt['name']} ({opt['model']})",
                value=str(opt["provider"]),
            )
            for opt in group_options
        ],
        default=default_provider_id,
        qmark="▸",
        pointer="›",
    ).ask()
    if provider_id is None:
        return None

    preset: ProviderOption = next(opt for opt in group_options if opt["provider"] == provider_id)
    provider = str(preset["provider"])
    default_model = str(preset.get("model", ""))
    requires_api_key = bool(preset.get("requires_api_key", True))
    preset_base_url = str(preset.get("base_url", "")) or None
    allow_base_url = bool(preset.get("allow_base_url", False) or preset_base_url)

    # Step 3: API Key (respect provider requirements)
    api_key_str: str | None = None
    if requires_api_key:
        existing_key = existing.api_key.get_secret_value() if existing.api_key and existing.provider == provider else ""
        api_key_str = questionary.password(
            "API Key:",
            default=existing_key,
            qmark="▸",
        ).ask()
        if not api_key_str:
            console.print("[yellow]Warning: No API key provided.[/yellow]")

    # Step 4: Model
    model_default = existing.model if existing.provider == provider and existing.model else default_model
    model = questionary.text(
        "Model name:",
        default=model_default or "gpt-4o-mini",
        qmark="▸",
    ).ask()
    if not model:
        model = default_model or "gpt-4o-mini"

    # Step 5: Base URL (for local/custom/proxy-capable endpoints)
    base_url: str | None = None
    if allow_base_url:
        default_url = existing.base_url if existing.provider == provider else preset_base_url
        base_url = questionary.text(
            "API Base URL:",
            default=default_url or "",
            qmark="▸",
        ).ask() or preset_base_url
    else:
        base_url = preset_base_url

    # Step 6: Default mode
    mode_choice = questionary.select(
        "Default operation mode:",
        choices=[
            questionary.Choice("Raw (forward only, no AI)", value="raw"),
            questionary.Choice("Hybrid (on-demand AI)", value="hybrid"),
            questionary.Choice("Agent (AI on every email)", value="agent"),
        ],
        default=existing.default_mode.value,
        qmark="▸",
        pointer="›",
    ).ask()
    if mode_choice is None:
        mode_choice = "hybrid"

    # Step 7: Output language
    lang_choice = questionary.select(
        "AI output language:",
        choices=[
            questionary.Choice("Auto-detect (match email language)", value="auto"),
            questionary.Choice("Chinese (中文)", value="zh"),
            questionary.Choice("English", value="en"),
            questionary.Choice("Japanese (日本語)", value="ja"),
        ],
        default=existing.language,
        qmark="▸",
        pointer="›",
    ).ask()
    if lang_choice is None:
        lang_choice = "auto"

    return AIConfig(
        enabled=True,
        provider=provider,
        api_key=SecretStr(api_key_str) if api_key_str else None,
        model=model,
        base_url=base_url,
        default_mode=OperationMode(mode_choice),
        language=lang_choice,
    )
