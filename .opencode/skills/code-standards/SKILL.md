---
name: code-standards
description: Use when implementing or reviewing MailBot Python changes that must follow project-specific architecture, callback, and logging conventions.
---

# Purpose
Keep MailBot code aligned with project architecture boundaries and runtime conventions.

# Superpowers Boundary
- Superpowers skills cover generic debugging, TDD, and completion verification workflows.
- This skill only defines MailBot-specific coding constraints.

# Project-Specific Rules
- Target Python 3.10+ with `from __future__ import annotations`.
- Keep module boundaries stable (`core/`, `interface/`, `utils/`) with high cohesion and low coupling.
- Use module-level loggers (`logging.getLogger("mailbot.<module>")`); do not log secrets.
- Keep callback handler signatures compatible with existing convention: `_cb_<action>(cq_id, data, chat_id, message_id)`.
- Use descriptive callback prefixes (for example `summ_`, `trans_`, `orig_`) and keep `_email_snapshot_cache` behavior compatible.
- Prefer `AppConfig` / `AccountConfig` models over ad-hoc dict schema changes.
- Preserve CLI/menu behavior unless explicitly approved.

# Review Checklist
- Interface/factory integration stays compatible with existing notifier/fetcher flow.
- Retry/timeout behavior remains bounded and uses shared settings.
- Threading/state transitions preserve existing safety assumptions.
- User-facing text, prompts, and bot messages remain English unless explicitly requested.

# Deliverables
- Code changes that satisfy the above constraints.
- Notes on verification commands run and outcomes.
