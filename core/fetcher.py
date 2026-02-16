"""
core/fetcher.py
~~~~~~~~~~~~~~~
IMAP fetcher with exponential backoff and fail-fast auth handling.

Responsibilities:
- Connect to IMAP and fetch unread messages
- Network errors: exponential backoff (5s → 10s → 30s)
- Auth errors: fail fast and mark account as error
- Track seen UIDs to avoid duplicate forwarding
"""

from __future__ import annotations

import logging
import time
from datetime import datetime, timezone
from typing import Callable

from imap_tools import AND, MailBox, MailBoxUnencrypted, MailboxLoginError

from core.models import (
    AccountConfig,
    AccountState,
    AccountStatus,
    EmailSnapshot,
)
from core.parser import parse_email

logger = logging.getLogger("mailbot.fetcher")

# Backoff seconds for retries
BACKOFF_BASE: list[int] = [5, 10, 30]
MAX_RETRIES_DEFAULT = 3


class EmailFetcher:
    """
    IMAP email fetcher bound to a single account.

    Responsibilities:
        1. Connect to IMAP
        2. Pull unread messages from configured folders
        3. Track seen UIDs
        4. Retry with backoff on network errors; fail fast on auth errors

    Args:
        account: account configuration
        max_retries: maximum retry attempts
        on_status_change: optional status change callback
    """

    def __init__(
        self,
        account: AccountConfig,
        max_retries: int = MAX_RETRIES_DEFAULT,
        on_status_change: Callable[[AccountStatus], None] | None = None,
    ) -> None:
        self._account = account
        self._max_retries = max_retries
        self._on_status_change = on_status_change

        # Record service start to filter out pre-start mail by timestamp
        self._started_at = datetime.now(timezone.utc)

        # Seen UID set to avoid duplicates
        self._seen_uids: set[str] = set()

        # Bootstrap flag: first poll only records existing unseen mail without sending
        self._bootstrap_done = False

        # Runtime status
        self._status = AccountStatus(
            name=account.name,
            email=account.email,
            state=AccountState.IDLE,
        )

    @property
    def account(self) -> AccountConfig:
        return self._account

    @property
    def status(self) -> AccountStatus:
        return self._status

    @property
    def is_errored(self) -> bool:
        """Return True if the account is in a terminal error state."""
        return self._status.state in (
            AccountState.AUTH_FAILED,
            AccountState.DISABLED,
        )

    def _update_status(self, **kwargs: object) -> None:
        """Update cached status and fire callback if provided."""
        self._status = self._status.model_copy(update=kwargs)
        if self._on_status_change:
            try:
                self._on_status_change(self._status)
            except Exception:
                logger.debug("Status change callback failed", exc_info=True)

    def fetch_new_emails(
        self,
        stop_check: Callable[[], bool] | None = None,
    ) -> list[EmailSnapshot]:
        """
        Fetch unread messages with retry logic.

        Args:
            stop_check: optional stop predicate

        Returns:
            List of new EmailSnapshot
        """
        if self.is_errored:
            logger.debug("Account [%s] is in error state, skip fetch", self._account.name)
            return []

        if not self._account.enabled:
            return []

        self._update_status(state=AccountState.RUNNING, error_message="")

        for attempt in range(self._max_retries):
            if stop_check and stop_check():
                self._update_status(state=AccountState.IDLE)
                return []

            try:
                snapshots = self._do_fetch()
                self._update_status(
                    state=AccountState.IDLE,
                    retry_count=0,
                )
                return snapshots

            except MailboxLoginError as e:
                # Auth failure → fail fast
                error_msg = f"Authentication failed: {e}"
                logger.error(
                    "Account [%s] authentication failed, stop retry — %s",
                    self._account.name,
                    error_msg,
                )
                self._update_status(
                    state=AccountState.AUTH_FAILED,
                    error_message=error_msg,
                )
                return []  # no retries on auth failure

            except (TimeoutError, OSError, ConnectionError) as e:
                wait_time = BACKOFF_BASE[min(attempt, len(BACKOFF_BASE) - 1)]
                logger.warning(
                    "Account [%s] connection failed (attempt %d/%d), retry in %ds — %s",
                    self._account.name,
                    attempt + 1,
                    self._max_retries,
                    wait_time,
                    str(e)[:100],
                )
                self._update_status(
                    state=AccountState.ERROR,
                    error_message=f"Network error, retrying ({attempt + 1}/{self._max_retries})",
                    retry_count=attempt + 1,
                )

                for _ in range(wait_time):
                    if stop_check and stop_check():
                        self._update_status(state=AccountState.IDLE)
                        return []
                    time.sleep(1)

            except Exception as e:
                logger.exception(
                    "Account [%s] encountered an unexpected error: %s",
                    self._account.name,
                    str(e)[:200],
                )
                self._update_status(
                    state=AccountState.ERROR,
                    error_message=f"Unexpected error: {str(e)[:100]}",
                )
                return []

        logger.error(
            "Account [%s] retries exhausted (%d), skipping",
            self._account.name,
            self._max_retries,
        )
        self._update_status(
            state=AccountState.ERROR,
            error_message=f"Failed after {self._max_retries} retries",
        )
        return []

    def _do_fetch(self) -> list[EmailSnapshot]:
        """
        Perform IMAP operations and return new snapshots.
        """
        snapshots: list[EmailSnapshot] = []
        password = self._account.password.get_secret_value()

        MailBoxCls = MailBox if self._account.use_ssl else MailBoxUnencrypted

        with MailBoxCls(
            host=self._account.imap_host,
            port=self._account.imap_port,
        ).login(
            username=self._account.email,
            password=password,
        ) as mailbox:
            logger.info("Connected [%s] (%s)", self._account.name, self._account.imap_host)

            # On first poll, pull all unseen to seed the seen-set so only post-start mail forwards
            fetch_limit = None if not self._bootstrap_done else 50

            for folder in self._account.folders:
                mailbox.folder.set(folder)
                logger.debug("Scanning folder: [%s]/%s", self._account.name, folder)

                for msg in mailbox.fetch(AND(seen=False), limit=fetch_limit, reverse=True):
                    uid = str(msg.uid)

                    if uid in self._seen_uids:
                        continue

                    # First run: record existing unseen mail but do not forward
                    if not self._bootstrap_done:
                        self._seen_uids.add(uid)
                        continue

                    msg_dt = self._normalize_dt(msg.date)
                    if msg_dt and msg_dt < self._started_at:
                        # Skip pre-start mail based on timestamp; mark seen to avoid repeats
                        self._seen_uids.add(uid)
                        continue

                    try:
                        snapshot = parse_email(msg, self._account)
                        snapshots.append(snapshot)
                        self._seen_uids.add(uid)
                    except Exception:
                        logger.exception("Failed to parse mail: [%s] UID=%s", self._account.name, uid)

        # Mark bootstrap complete after first poll
        if not self._bootstrap_done:
            self._bootstrap_done = True

        if snapshots:
            logger.info(
                "Account [%s] fetched %d new emails",
                self._account.name,
                len(snapshots),
            )
            self._update_status(
                forwarded_count=self._status.forwarded_count + len(snapshots),
            )
        else:
            logger.debug("Account [%s] no new mail", self._account.name)

        from datetime import datetime
        self._update_status(last_check=datetime.now())

        return snapshots

    def _normalize_dt(self, dt: datetime | None) -> datetime | None:
        """Return timezone-aware datetime or None."""
        if dt is None:
            return None
        if dt.tzinfo is None:
            return dt.replace(tzinfo=timezone.utc)
        return dt.astimezone(timezone.utc)

    def reset_error(self) -> None:
        """Clear error state to allow fetching again."""
        if self._status.state in (AccountState.ERROR, AccountState.AUTH_FAILED):
            logger.info("Account [%s] error state reset", self._account.name)
            self._update_status(
                state=AccountState.IDLE,
                error_message="",
                retry_count=0,
            )

    def clear_seen(self) -> None:
        """Clear the cache of seen UIDs."""
        count = len(self._seen_uids)
        self._seen_uids.clear()
        logger.info("Account [%s] cleared %d seen records", self._account.name, count)
