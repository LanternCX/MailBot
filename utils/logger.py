"""
utils/logger.py
~~~~~~~~~~~~~~~
Rich-based logging setup: rotating file + styled console output.
"""

from __future__ import annotations

import logging
import logging.handlers
from pathlib import Path

from rich.console import Console
from rich.logging import RichHandler

# ── Constants ──

LOG_DIR = Path(__file__).resolve().parent.parent / "logs"
LOG_FILE = LOG_DIR / "mailbot.log"
MAX_BYTES = 5 * 1024 * 1024  # 5 MB
BACKUP_COUNT = 3
FILE_FORMAT = "%(asctime)s | %(levelname)-8s | %(name)-20s | %(message)s"

console = Console()
ACTIVE_LOG_LEVEL = logging.INFO


def get_active_log_level() -> int:
    return ACTIVE_LOG_LEVEL


def adopt_dependency_logger(name: str, level: int, force_handlers: bool = False) -> None:
    """Align dependency logger level with project level; optionally force handlers/propagation."""
    dep_logger = logging.getLogger(name)
    dep_logger.setLevel(level)
    if force_handlers:
        dep_logger.handlers.clear()
        dep_logger.propagate = True


def adopt_dependency_loggers(
    prefixes: tuple[str, ...],
    level: int | None = None,
    force_handlers: bool = False,
) -> None:
    """Apply dependency logger alignment for multiple prefixes (handles children)."""
    eff_level = level if level is not None else get_active_log_level()
    for name in prefixes:
        adopt_dependency_logger(name, eff_level, force_handlers)

    # Also align any existing child loggers (e.g., LiteLLM.http_handler)
    for name, obj in logging.root.manager.loggerDict.items():
        if not isinstance(obj, logging.Logger):
            continue
        if any(name == prefix or name.startswith(f"{prefix}.") for prefix in prefixes):
            obj.setLevel(eff_level)
            if force_handlers:
                obj.handlers.clear()
                obj.propagate = True


def setup_logging(level: str = "INFO") -> None:
    """
    Initialize logging once at startup.

    - RotatingFileHandler  → logs/mailbot.log (DEBUG)
    - RichHandler          → stderr with colors (user-chosen level)
    """
    global ACTIVE_LOG_LEVEL

    LOG_DIR.mkdir(parents=True, exist_ok=True)
    log_level = getattr(logging, level.upper(), logging.INFO)
    ACTIVE_LOG_LEVEL = log_level

    root = logging.getLogger()
    root.setLevel(logging.DEBUG)
    root.handlers.clear()

    # File handler (always DEBUG)
    fh = logging.handlers.RotatingFileHandler(
        filename=str(LOG_FILE),
        maxBytes=MAX_BYTES,
        backupCount=BACKUP_COUNT,
        encoding="utf-8",
    )
    fh.setLevel(logging.DEBUG)
    fh.setFormatter(logging.Formatter(FILE_FORMAT))
    root.addHandler(fh)

    # Console handler (Rich)
    rh = RichHandler(
        console=console,
        level=log_level,
        show_path=False,
        show_time=True,
        rich_tracebacks=True,
        markup=True,
    )
    rh.setFormatter(logging.Formatter("%(message)s", datefmt="[%H:%M:%S]"))
    root.addHandler(rh)

    # Ensure dependency loggers (e.g., LiteLLM) align with current level (no forced handlers)
    adopt_dependency_loggers(("LiteLLM",), level=log_level, force_handlers=False)

    logging.getLogger("mailbot").info(
        "Logging ready — level=%s  file=%s", level, LOG_FILE,
    )
