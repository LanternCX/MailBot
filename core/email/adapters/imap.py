"""IMAP adapter bridging existing fetcher implementation."""

from __future__ import annotations

from core.email.base import EmailSourceBase
from core.fetcher import EmailFetcher
from core.models import EmailSnapshot


class ImapEmailSource(EmailSourceBase):
    def __init__(self, fetcher: EmailFetcher) -> None:
        self._fetcher = fetcher

    def poll_new_emails(self) -> list[EmailSnapshot]:
        return self._fetcher.fetch_new_emails()
