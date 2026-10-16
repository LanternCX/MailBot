---
name: code-standards
description: Use when implementing or reviewing MailBot Go changes that must follow project-specific architecture, callback, persistence, and logging conventions.
---

# Purpose
Keep MailBot code aligned with project architecture boundaries and runtime conventions.

# Superpowers Boundary
- Superpowers skills cover generic debugging, TDD, and completion verification workflows.
- This skill only defines MailBot-specific coding constraints.

# Project-Specific Rules
- Target the Go version in `go.mod` and format with `gofmt`.
- Keep entrypoints in `cmd/` and runtime behavior in `internal/mailbot/`.
- Use `log/slog`; do not log secrets or email bodies.
- Keep Telegram callback prefixes (`summ_`, `trans_`, `orig_`, `settings_`, `lang_`, `mode_`, `rules_`) compatible; bind cached emails to their allowed target.
- Use typed `Config` / `Account` values and preserve unrelated JSON fields when persisting runtime settings.
- Commit delivery jobs and IMAP checkpoints together before marking mail read. Keep per-target delivery and multipart progress durable.
- Use shared network transports with bounded operations and context cancellation; do not mutate global sockets or environment proxies.
- Preserve CLI/menu behavior unless explicitly approved.

# Review Checklist
- Interface/factory integration stays compatible with existing notifier/fetcher flow.
- Retry/timeout behavior remains bounded and uses shared settings.
- Goroutines, state transitions, and shutdown preserve cancellation and race safety.
- User-facing text, prompts, and bot messages remain English unless explicitly requested.

# Deliverables
- Code changes that satisfy the above constraints.
- Notes on verification commands run and outcomes.
