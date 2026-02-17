#!/usr/bin/env python3
"""
main.py
~~~~~~~
MailBot entrypoint — menu-driven interactive CLI.

Usage:
    python main.py                  # interactive menu (default)
    python main.py -c path.json     # custom config path
    python main.py --headless       # run service without menu (requires config)
"""

from __future__ import annotations

import argparse
import logging
import signal
import sys
import time
from pathlib import Path

# Add project root to sys.path
PROJECT_ROOT = Path(__file__).resolve().parent
if str(PROJECT_ROOT) not in sys.path:
    sys.path.insert(0, str(PROJECT_ROOT))

from utils.logger import setup_logging

logger = logging.getLogger("mailbot.main")

def default_config_path() -> Path:
    """Config path in the current working directory."""
    return Path.cwd() / "config.json"


def parse_args() -> argparse.Namespace:
    """Parse CLI arguments."""
    parser = argparse.ArgumentParser(
        prog="MailBot",
        description="IMAP-to-Telegram mail forwarder (interactive CLI)",
    )
    parser.add_argument(
        "-c", "--config",
        type=str,
        default=str(default_config_path()),
        help="Config file path (default: config.json in current directory)",
    )
    parser.add_argument(
        "--headless",
        action="store_true",
        help="Run service without menu (requires existing config)",
    )
    return parser.parse_args()


def run_headless(config_path: Path) -> None:
    """Headless mode — start service, block until Ctrl+C."""
    from core.manager import ServiceManager
    from core.models import AppConfig
    from utils.helpers import apply_global_proxy

    if not config_path.exists():
        print(f"Error: Config not found — {config_path}")
        print("Run without --headless to use the setup wizard.")
        sys.exit(1)

    try:
        config = AppConfig.load(config_path)
    except Exception as exc:
        print(f"Error: Invalid config — {exc}")
        sys.exit(1)

    apply_global_proxy(config.proxy)
    setup_logging(level=config.log_level)

    manager = ServiceManager(config)
    manager.start()
    logger.info("Headless mode — Ctrl+C to stop")

    stop = False

    def _sigint(sig, frame):  # noqa: ANN001
        nonlocal stop
        stop = True

    signal.signal(signal.SIGINT, _sigint)
    try:
        while not stop:
            time.sleep(0.5)
    finally:
        manager.stop()
        logger.info("Service stopped")


def run_interactive(config_path: Path) -> None:
    """Interactive menu mode."""
    from interface.menu import main_menu
    from core.models import AppConfig
    from utils.helpers import apply_global_proxy

    # Ensure proxy is applied before network ops (menus/tests)
    try:
        cfg = AppConfig.load(config_path)
    except Exception:
        cfg = AppConfig()
    apply_global_proxy(cfg.proxy)

    setup_logging(level="INFO")
    main_menu(config_path)


def main() -> None:
    """Entry point."""
    args = parse_args()
    config_path = Path(args.config)

    if args.headless:
        run_headless(config_path)
    else:
        run_interactive(config_path)


if __name__ == "__main__":
    main()
