"""
core/models.py
~~~~~~~~~~~~~~
Pydantic data models for configuration and runtime state.

Includes:
- AccountConfig: IMAP account configuration
- NotifierConfig: notifier adapter configuration
- AppConfig: global application settings
- EmailSnapshot: immutable email snapshot
- AccountStatus: runtime state per account
"""

from __future__ import annotations

import json
from datetime import datetime
from enum import Enum
from pathlib import Path
from typing import Any

from pydantic import BaseModel, Field, SecretStr, field_validator


# ──────────────────────────────────────────────
#  Enums
# ──────────────────────────────────────────────

class AccountState(str, Enum):
    """Runtime state for a single account."""
    IDLE = "idle"
    RUNNING = "running"
    ERROR = "error"
    AUTH_FAILED = "auth_failed"
    DISABLED = "disabled"


class ServiceState(str, Enum):
    """Overall service state."""
    STOPPED = "stopped"
    RUNNING = "running"
    STOPPING = "stopping"


# ──────────────────────────────────────────────
#  Persistent configuration models
# ──────────────────────────────────────────────

class AccountConfig(BaseModel):
    """Single IMAP account configuration."""
    name: str = Field(..., description="Display name, e.g. 'Work Gmail'")
    email: str = Field(..., description="Email address")
    password: SecretStr = Field(..., description="IMAP app password / token")
    imap_host: str = Field(..., description="IMAP server host, e.g. imap.gmail.com")
    imap_port: int = Field(default=993, description="IMAP port, default 993 (SSL)")
    use_ssl: bool = Field(default=True, description="Use SSL connection")
    folders: list[str] = Field(default=["INBOX"], description="Folders to monitor")
    enabled: bool = Field(default=True, description="Whether this account is enabled")
    web_url: str = Field(default="", description="Optional webmail base URL for links")

    @field_validator("imap_port")
    @classmethod
    def validate_port(cls, v: int) -> int:
        if not (1 <= v <= 65535):
            raise ValueError(f"Port must be between 1-65535, got: {v}")
        return v


class TelegramNotifierConfig(BaseModel):
    """Telegram notifier settings."""
    bot_token: SecretStr = Field(..., description="Telegram Bot Token")
    chat_id: str = Field(..., description="Target Chat ID")
    api_base: str = Field(
        default="https://api.telegram.org",
        description="Telegram API base URL (proxy friendly)",
    )
    timeout: int = Field(default=30, description="HTTP timeout in seconds")


class NotifierConfig(BaseModel):
    """Unified notifier configuration."""
    type: str = Field(..., description="Notifier type: telegram / discord / slack ...")
    enabled: bool = Field(default=True, description="Whether this notifier is enabled")
    telegram: TelegramNotifierConfig | None = Field(default=None)

    @field_validator("type")
    @classmethod
    def validate_type(cls, v: str) -> str:
        supported = {"telegram"}
        if v not in supported:
            raise ValueError(f"Unsupported notifier type: {v}, supported: {supported}")
        return v


class AppConfig(BaseModel):
    """Global application configuration."""
    poll_interval: int = Field(
        default=60,
        ge=10,
        description="Polling interval in seconds (min 10)",
    )
    max_retries: int = Field(default=3, ge=1, description="Max retry attempts for network errors")
    log_level: str = Field(default="INFO", description="Log level: DEBUG/INFO/WARNING/ERROR")
    accounts: list[AccountConfig] = Field(default_factory=list, description="IMAP accounts")
    notifiers: list[NotifierConfig] = Field(default_factory=list, description="Notifiers list")

    @field_validator("log_level")
    @classmethod
    def validate_log_level(cls, v: str) -> str:
        v = v.upper()
        if v not in {"DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"}:
            raise ValueError(f"Invalid log level: {v}")
        return v

    @classmethod
    def load(cls, path: str | Path) -> "AppConfig":
        """Load configuration from a JSON file."""
        config_path = Path(path)
        if not config_path.exists():
            raise FileNotFoundError(f"Config file not found: {config_path}")
        raw: dict[str, Any] = json.loads(config_path.read_text(encoding="utf-8"))
        return cls.model_validate(raw)

    def save(self, path: str | Path) -> None:
        """Persist configuration to a JSON file."""
        config_path = Path(path)
        data = self.model_dump(mode="python")
        # Ensure secrets are stored as plain strings
        for acc in data.get("accounts", []):
            pwd = acc.get("password")
            if isinstance(pwd, SecretStr):
                acc["password"] = pwd.get_secret_value()
        for notifier in data.get("notifiers", []):
            tg = notifier.get("telegram")
            if tg and isinstance(tg.get("bot_token"), SecretStr):
                tg["bot_token"] = tg["bot_token"].get_secret_value()
        config_path.write_text(
            json.dumps(data, indent=2, ensure_ascii=False),
            encoding="utf-8",
        )


# ──────────────────────────────────────────────
#  Runtime models
# ──────────────────────────────────────────────

class EmailSnapshot(BaseModel):
    """Immutable snapshot of a fetched email."""
    uid: str = Field(..., description="Email UID")
    account_name: str = Field(..., description="Account display name")
    subject: str = Field(default="(No subject)", description="Email subject")
    sender: str = Field(default="", description="From header")
    date: datetime | None = Field(default=None, description="Email date")
    body_text: str = Field(default="", description="Cleaned plain text body")
    body_html: str = Field(default="", description="Original HTML body")
    web_link: str = Field(default="", description="Webmail link")

    class Config:
        frozen = True  # snapshots should be immutable


class AccountStatus(BaseModel):
    """Runtime status for an account."""
    name: str
    email: str
    state: AccountState = AccountState.IDLE
    last_check: datetime | None = None
    error_message: str = ""
    forwarded_count: int = 0
    retry_count: int = 0
