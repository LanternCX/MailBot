"""
core/parser.py
~~~~~~~~~~~~~~
Email parsing utilities: clean HTML and build web links.

Responsibilities:
- Clean HTML body to plain text
- Generate webmail links
- Normalize metadata into EmailSnapshot
"""

from __future__ import annotations

import logging
import re
from datetime import datetime
from typing import TYPE_CHECKING

from bs4 import BeautifulSoup

if TYPE_CHECKING:
    from imap_tools import MailMessage

from core.models import AccountConfig, EmailSnapshot

logger = logging.getLogger("mailbot.parser")


def clean_html(html: str) -> str:
    """
    Clean HTML into readable plain text.

    Steps:
        1. Remove script/style/head/meta/link tags
        2. Extract text
        3. Collapse extra whitespace and blank lines

    Args:
        html: raw HTML string

    Returns:
        Cleaned plain text
    """
    if not html or not html.strip():
        return ""

    try:
        soup = BeautifulSoup(html, "html.parser")

        for tag in soup(["script", "style", "head", "meta", "link"]):
            tag.decompose()

        for br in soup.find_all("br"):
            br.replace_with("\n")
        for block_tag in soup.find_all(["p", "div", "tr", "li"]):
            block_tag.insert_before("\n")
            block_tag.insert_after("\n")

        text = soup.get_text(separator=" ")

        text = re.sub(r"[ \t]+", " ", text)  # collapse multiple spaces
        text = re.sub(r"\n\s*\n", "\n\n", text)  # collapse blank lines
        text = text.strip()

        return text

    except Exception:
        logger.exception("Failed to clean HTML, returning stripped text")
        return re.sub(r"<[^>]+>", "", html).strip()


def generate_web_link(
    account: AccountConfig,
    uid: str,
) -> str:
    """
    Build a webmail link based on account and UID.

    Supported heuristics:
        - Gmail:  https://mail.google.com/mail/u/0/#inbox/<uid>
        - Outlook: https://outlook.live.com/mail/0/inbox/id/<uid>
        - Custom: uses account.web_url as prefix

    Args:
        account: account config
        uid: message UID

    Returns:
        Web link string or empty if unknown
    """
    if account.web_url:
        base = account.web_url.rstrip("/")
        return f"{base}/{uid}"

    host = account.imap_host.lower()

    if "gmail" in host or "google" in host:
        return f"https://mail.google.com/mail/u/0/#inbox/{uid}"
    elif "outlook" in host or "hotmail" in host:
        return f"https://outlook.live.com/mail/0/inbox/id/{uid}"
    elif "qq.com" in host:
        return "https://mail.qq.com/"
    elif "163.com" in host or "126.com" in host:
        return f"https://mail.163.com/"

    return ""


def parse_email(
    msg: "MailMessage",
    account: AccountConfig,
) -> EmailSnapshot:
    """
    Convert imap_tools MailMessage to EmailSnapshot.

    Args:
        msg: imap_tools message object
        account: account config

    Returns:
        EmailSnapshot
    """
    sender = str(msg.from_ or "")

    subject = msg.subject or "(No subject)"

    mail_date: datetime | None = None
    if msg.date:
        mail_date = msg.date

    body_html = msg.html or ""
    body_text = msg.text or ""

    if not body_text and body_html:
        body_text = clean_html(body_html)

    web_link = generate_web_link(account, str(msg.uid))

    snapshot = EmailSnapshot(
        uid=str(msg.uid),
        account_name=account.name,
        subject=subject,
        sender=sender,
        date=mail_date,
        body_text=body_text,
        body_html=body_html,
        web_link=web_link,
    )

    logger.debug(
        "Parsed email: [%s] %s ← %s",
        account.name,
        subject[:50],
        sender,
    )
    return snapshot
