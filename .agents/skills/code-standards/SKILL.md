---
name: code-standards
description: Enforce MailBot Python code style, refactoring guardrails, architecture health, and logging quality.
---

# Purpose
Keep the codebase consistent with open-source Python norms (PEP 8/PEP 257/typing), project patterns, maintainable architecture, and production-grade logging.

# When to Use
- Writing new Python modules or CLI flows.
- Refactoring service orchestration, fetchers, notifiers, or helpers.
- Reviewing contributions for style, safety, observability, and architecture integrity.

# Style & Structure Rules
- Target Python 3.10+; include `from __future__ import annotations` at module top.
- Type hints mandatory; prefer explicit return types. Avoid `Any` unless unavoidable.
- Use module-level loggers via `logging.getLogger("mailbot.<module>")`; avoid `print` except user-facing CLI/wizard interactions that intentionally bypass logging.
- Logging must be comprehensive: contextual messages for lifecycle, retries, and failures; structured fields where sensible; never log secrets.
- Keep functions short and single-purpose; extract helpers for repeated blocks.
- Docstrings: concise summary; note side effects or non-obvious args/returns.
- Comments: English only; reserve for non-trivial logic/rationale.
- Error handling: catch narrow exceptions; log with context; avoid silent failures.
- Concurrency: guard shared state with locks; use events for stop signals; avoid races.
- Imports: stdlib → third-party → local; drop unused imports; avoid circulars.
- Configuration: prefer `AppConfig`/`AccountConfig` models; no ad-hoc dict schemas.
- CLI UX: use Rich; prompts explicit and safe defaults.
- Architecture: preserve clear module boundaries; aim for high cohesion / low coupling to keep the repo maintainable and extensible.

# Code Review Checklist
- Interfaces: new notifiers/fetchers follow factories and respect `enabled` flags.
- Logging: lifecycle and error paths covered; no sensitive data; verbosity appropriate.
- Retries/timeouts: network calls bounded; reuse global settings where possible.
- Resource safety: threads/events shut down cleanly; connections/clients closed.
- Tests/manual: add/adjust smoke steps (e.g., `python main.py --headless -c config.json` with sample config or wizard flow).
- Lint/readability: naming, spacing, removal of dead code.
- Post-change sanity: run analyzers/error checks until clean; do not finish a response with unresolved errors.

# Refactor Guardrails
- Preserve public behaviors and CLI contract (`main.py` args, menu flow) unless approved.
- Do not break config schema; if unavoidable, add migration defaults and notes.
- Keep banner/version constants centralized; avoid literal duplication.
- Maintain thread-safety around shared counters or state transitions.

# Deliverables
- Updated code conforming to the above.
- Notes on testing performed (commands and outcomes).
- Confirmation that error checks show no remaining issues.
- If docs impacted, hand off to `doc-maintainer` skill.
