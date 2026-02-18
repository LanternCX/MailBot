"""
core/rules.py
~~~~~~~~~~~~~
Markdown-based persona & rules manager.

Stores user-defined natural language directives in a Markdown file.
Each line is a numbered rule injected into the AI system prompt at runtime.
"""

from __future__ import annotations

import logging
import re
from pathlib import Path

logger = logging.getLogger("mailbot.rules")

DEFAULT_RULES_PATH = Path("rules.md")


class RulesManager:
    """
    Manages persona rules stored in a Markdown file.

    File format:
        1. Rule text here
        2. Another rule
        ...
    """

    def __init__(self, path: Path | str = DEFAULT_RULES_PATH) -> None:
        self._path = Path(path)

    @property
    def path(self) -> Path:
        return self._path

    # ──────────────────────────────────────────────
    #  Read
    # ──────────────────────────────────────────────

    def load_rules(self) -> list[str]:
        """Return all rules as a list of strings (without numbering)."""
        if not self._path.exists():
            return []

        lines = self._path.read_text(encoding="utf-8").strip().splitlines()
        rules: list[str] = []
        for line in lines:
            # Strip leading number + dot: "1. some text" → "some text"
            cleaned = re.sub(r"^\d+\.\s*", "", line.strip())
            if cleaned:
                rules.append(cleaned)
        return rules

    def load_raw(self) -> str:
        """Return the raw Markdown content (for display)."""
        if not self._path.exists():
            return ""
        return self._path.read_text(encoding="utf-8").strip()

    # ──────────────────────────────────────────────
    #  Write helpers
    # ──────────────────────────────────────────────

    def _save_rules(self, rules: list[str]) -> None:
        """Persist rules list back to Markdown with numbering."""
        lines = [f"{i}. {rule}" for i, rule in enumerate(rules, start=1)]
        self._path.write_text("\n".join(lines) + "\n", encoding="utf-8")
        logger.info("Rules saved: %d entries → %s", len(rules), self._path)

    # ──────────────────────────────────────────────
    #  Add / Delete
    # ──────────────────────────────────────────────

    def add_rule(self, text: str) -> int:
        """
        Append a new rule. Returns the new rule count.

        Args:
            text: rule text (without numbering)
        """
        rules = self.load_rules()
        rules.append(text.strip())
        self._save_rules(rules)
        return len(rules)

    def delete_rule(self, index: int) -> bool:
        """
        Delete a rule by 1-based index. Returns True on success.

        Args:
            index: 1-based rule number
        """
        rules = self.load_rules()
        if index < 1 or index > len(rules):
            return False
        rules.pop(index - 1)
        self._save_rules(rules)
        return True

    def clear_rules(self) -> None:
        """Remove all rules."""
        self._save_rules([])

    # ──────────────────────────────────────────────
    #  Prompt injection
    # ──────────────────────────────────────────────

    def as_prompt_block(self) -> str | None:
        """
        Return rules formatted as a prompt block for system message injection.
        Returns None if no rules exist.
        """
        rules = self.load_rules()
        if not rules:
            return None

        lines = ["[User Preferences]"]
        for i, rule in enumerate(rules, start=1):
            lines.append(f"{i}. {rule}")
        return "\n".join(lines)
