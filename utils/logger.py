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


def setup_logging(level: str = "INFO") -> None:
    """
    Initialize logging once at startup.

    - RotatingFileHandler  → logs/mailbot.log (DEBUG)
    - RichHandler          → stderr with colors (user-chosen level)
    """
    LOG_DIR.mkdir(parents=True, exist_ok=True)
    log_level = getattr(logging, level.upper(), logging.INFO)

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

    logging.getLogger("mailbot").info(
        "Logging ready — level=%s  file=%s", level, LOG_FILE,
    )
