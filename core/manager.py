"""
core/manager.py
~~~~~~~~~~~~~~~
Service orchestration and threading.

Responsibilities:
- Initialize components (Config → Notifiers → Fetchers)
- Run main loop on a daemon thread
- Loop: Fetch → Parse → Notify → Sleep
- Expose runtime stats
"""

from __future__ import annotations

import logging
import threading
import time
from datetime import datetime
from typing import Callable

from core.fetcher import EmailFetcher
from core.models import (
    AccountConfig,
    AccountState,
    AccountStatus,
    AIConfig,
    AppConfig,
    EmailSnapshot,
    NotifierConfig,
    OperationMode,
    ServiceState,
    TelegramNotifierConfig,
)
from core.notifiers.base import BaseNotifier
from core.notifiers.telegram import TelegramNotifier

logger = logging.getLogger("mailbot.manager")


class ServiceManager:
    """
    Lifecycle manager for fetchers and notifiers.

    Runs the main loop on a daemon thread.
    """

    def __init__(
        self,
        config: AppConfig,
        on_status_change: Callable[[AccountStatus], None] | None = None,
        on_email_forwarded: Callable[[EmailSnapshot], None] | None = None,
    ) -> None:
        self._config = config
        self._on_status_change = on_status_change
        self._on_email_forwarded = on_email_forwarded

        # Service state
        self._state = ServiceState.STOPPED
        self._state_lock = threading.Lock()

        # Thread control
        self._thread: threading.Thread | None = None
        self._stop_event = threading.Event()

        # Stats
        self._start_time: datetime | None = None
        self._total_forwarded: int = 0
        self._total_forwarded_lock = threading.Lock()

        # Components
        self._fetchers: list[EmailFetcher] = []
        self._notifiers: list[BaseNotifier] = []

        # Bot handler (for Telegram commands/callbacks)
        self._bot_handler = None

        # Initialize
        self._init_notifiers()
        self._init_fetchers()
        self._init_bot()

    # ──────────────────────────────────────────────
    #  Properties
    # ──────────────────────────────────────────────

    @property
    def state(self) -> ServiceState:
        with self._state_lock:
            return self._state

    @property
    def is_running(self) -> bool:
        return self.state == ServiceState.RUNNING

    @property
    def start_time(self) -> datetime | None:
        return self._start_time

    @property
    def total_forwarded(self) -> int:
        with self._total_forwarded_lock:
            return self._total_forwarded

    @property
    def account_count(self) -> int:
        return len([f for f in self._fetchers if f.account.enabled])

    @property
    def account_statuses(self) -> list[AccountStatus]:
        return [f.status for f in self._fetchers]

    @property
    def config(self) -> AppConfig:
        return self._config

    # ──────────────────────────────────────────────
    #  Initialization
    # ──────────────────────────────────────────────

    def _init_notifiers(self) -> None:
        """Initialize notifiers from config."""
        self._notifiers.clear()
        for nc in self._config.notifiers:
            if not nc.enabled:
                continue
            notifier = self._create_notifier(nc)
            if notifier:
                self._notifiers.append(notifier)
                logger.info("Notifier registered: %s", notifier.name)

    def _create_notifier(self, nc: NotifierConfig) -> BaseNotifier | None:
        """Factory for notifier instances."""
        if nc.type == "telegram" and nc.telegram:
            return TelegramNotifier(nc.telegram)
        logger.warning("Unsupported notifier type: %s", nc.type)
        return None

    def _init_fetchers(self) -> None:
        """Initialize fetchers from config."""
        self._fetchers.clear()
        for acc in self._config.accounts:
            fetcher = EmailFetcher(
                account=acc,
                max_retries=self._config.max_retries,
                on_status_change=self._on_status_change,
            )
            self._fetchers.append(fetcher)
            logger.info("Fetcher registered: [%s] %s", acc.name, acc.email)

    def _init_bot(self) -> None:
        """Initialize the Telegram bot handler for commands/callbacks."""
        from core.bot import TelegramBotHandler

        ai_config = self._config.ai

        # Find the first enabled Telegram notifier
        tg_notifier: TelegramNotifier | None = None
        for n in self._notifiers:
            if isinstance(n, TelegramNotifier):
                tg_notifier = n
                break

        if not tg_notifier:
            logger.debug("No Telegram notifier — bot handler disabled")
            self._bot_handler = None
            return

        default_mode = ai_config.default_mode if ai_config.enabled else OperationMode.RAW

        self._bot_handler = TelegramBotHandler(
            notifier=tg_notifier,
            ai_config=ai_config,
            default_mode=default_mode,
        )
        logger.info("Bot handler initialized (default_mode=%s)", default_mode.value)

    # ──────────────────────────────────────────────
    #  Service lifecycle
    # ──────────────────────────────────────────────

    def start(self) -> None:
        """Start service on a daemon thread."""
        with self._state_lock:
            if self._state == ServiceState.RUNNING:
                logger.warning("Service already running")
                return
            self._state = ServiceState.RUNNING

        self._stop_event.clear()
        self._start_time = datetime.now()

        self._thread = threading.Thread(
            target=self._main_loop,
            name="MailBot-Worker",
            daemon=True,
        )
        self._thread.start()

        # Start bot handler for commands/callbacks
        if self._bot_handler:
            self._bot_handler.start()

        logger.info("MailBot service started")

    def stop(self) -> None:
        """Stop service and wait for the worker thread."""
        with self._state_lock:
            if self._state != ServiceState.RUNNING:
                return
            self._state = ServiceState.STOPPING

        logger.info("Stopping MailBot service...")
        self._stop_event.set()

        # Stop bot handler
        if self._bot_handler:
            self._bot_handler.stop()

        if self._thread and self._thread.is_alive():
            self._thread.join(timeout=30)

        with self._state_lock:
            self._state = ServiceState.STOPPED

        logger.info("MailBot service stopped")

    def reload_config(self, config: AppConfig) -> None:
        """Reload configuration (service will restart if it was running)."""
        was_running = self.is_running
        if was_running:
            self.stop()

        self._config = config
        self._init_notifiers()
        self._init_fetchers()
        self._init_bot()
        logger.info("Configuration reloaded")

        if was_running:
            self.start()

    # ──────────────────────────────────────────────
    #  Main loop
    # ──────────────────────────────────────────────

    def _main_loop(self) -> None:
        """Daemon main loop: Fetch → Notify → Sleep."""
        logger.info(
            "Main loop start — %d accounts, %d notifiers, interval %ds",
            self.account_count,
            len(self._notifiers),
            self._config.poll_interval,
        )

        while not self._stop_event.is_set():
            try:
                self._poll_cycle()
            except Exception:
                logger.exception("Unexpected error in main loop")

            for _ in range(self._config.poll_interval):
                if self._stop_event.is_set():
                    break
                time.sleep(1)

        logger.info("Main loop exited")

    def _poll_cycle(self) -> None:
        """One polling cycle."""
        all_snapshots: list[EmailSnapshot] = []

        for fetcher in self._fetchers:
            if self._stop_event.is_set():
                break

            snapshots = fetcher.fetch_new_emails(
                stop_check=self._stop_event.is_set,
            )
            all_snapshots.extend(snapshots)

        if not all_snapshots:
            return

        for snapshot in all_snapshots:
            if self._stop_event.is_set():
                break
            self._dispatch_notification(snapshot)

    def _dispatch_notification(self, snapshot: EmailSnapshot) -> None:
        """Send snapshots to all notifiers with mode-aware processing."""
        success_count = 0

        # Determine current mode and run AI if needed
        current_mode = OperationMode.RAW
        ai_result = None

        if self._bot_handler:
            current_mode = self._bot_handler.mode

            # Cache email for potential hybrid callback
            if self._bot_handler:
                self._bot_handler.cache_email(snapshot, ai_result.source_language if ai_result else None)

            # Agent mode: always run AI
            if current_mode == OperationMode.AGENT:
                from core.ai import analyze_email
                rules_block = self._bot_handler.rules_block if self._bot_handler else None
                ai_result = analyze_email(
                    subject=snapshot.subject,
                    sender=snapshot.sender,
                    body=snapshot.body_text,
                    config=self._config.ai,
                    rules_block=rules_block,
                )

        for notifier in self._notifiers:
            try:
                if isinstance(notifier, TelegramNotifier):
                    target_lang = self._bot_handler.language if self._bot_handler else None
                    ok = notifier.send_with_mode(snapshot, current_mode, ai_result, target_lang)
                else:
                    ok = notifier.send(snapshot)

                if ok:
                    success_count += 1
                else:
                    logger.warning(
                        "Notifier [%s] failed to send: %s",
                        notifier.name,
                        snapshot.subject[:50],
                    )
            except Exception:
                logger.exception(
                    "Notifier [%s] raised an exception: %s",
                    notifier.name,
                    snapshot.subject[:50],
                )

        if success_count > 0:
            with self._total_forwarded_lock:
                self._total_forwarded += 1
            logger.info(
                "Forwarded: [%s] %s (%d/%d notifiers)",
                snapshot.account_name,
                snapshot.subject[:50],
                success_count,
                len(self._notifiers),
            )
            if self._on_email_forwarded:
                try:
                    self._on_email_forwarded(snapshot)
                except Exception:
                    logger.debug("Forwarded callback failed", exc_info=True)

    # ──────────────────────────────────────────────
    #  Uptime helpers
    # ──────────────────────────────────────────────

    def get_uptime(self) -> str:
        """Return formatted uptime string."""
        if not self._start_time or not self.is_running:
            return "00:00:00"
        delta = datetime.now() - self._start_time
        hours, remainder = divmod(int(delta.total_seconds()), 3600)
        minutes, seconds = divmod(remainder, 60)
        return f"{hours:02d}:{minutes:02d}:{seconds:02d}"
