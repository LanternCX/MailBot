# Repository Guidelines

## Project Structure & Module Organization
- `cmd/mailbot/` is the CLI entry point and service launcher.
- `internal/mailbot/` contains configuration, CLI dialogs, IMAP/MIME, the SQLite queue, Telegram, AI providers, proxy networking, and logging.
- `cmd/package/` builds release archives; `deploy/` contains the systemd unit.
- `docs/` includes setup and configuration guides; images live under `docs/img/`.
- Tests live beside Go source as `*_test.go` and use local protocol servers.
- `logs/` is generated beside the active configuration file at runtime.

## Build, Test, and Development Commands
- Use Go 1.26 or newer; runtime builds have no Python or CGO dependency.
- `go run ./cmd/mailbot` opens the interactive menu.
- `go run ./cmd/mailbot --headless -c config.json` starts the service.
- `go run ./cmd/package --clean --variant macos-arm64 --tag dev` builds a release archive.

## Coding Style & Naming Conventions
- Use Go types, context cancellation, and explicit error handling; keep functions single-purpose.
- Format with `gofmt`; use `log/slog` outside CLI prompts.
- Persist delivery state before advancing IMAP checkpoints or marking messages read.
- Route network operations through the shared network implementation; keep deadlines and proxy-side DNS intact.
- Keep user-facing text in English and avoid logging secrets.

## Testing Guidelines
- `go test -race -timeout 60s ./...` runs unit, protocol, CLI, and integration tests without production credentials.
- `go vet ./...` checks static issues.
- Keep coverage focused on user-visible behavior, recovery, and protocol compatibility. See `docs/dev/development_zh.md` for the feature-to-test mapping.

## Commit & Pull Request Guidelines
- Follow Conventional Commits as seen in history: `feat:`, `fix:`, `docs:`.
- PRs should include a brief summary, tests run, and any config or docs changes.
- If you touch user-facing behavior, update `docs/en/` (and `README_ZH.md` when applicable).

## Skill Governance (Superpowers + Project Overrides)
- Superpowers skills provide the default process workflow.
- Project-specific overrides live in `.opencode/skills/` and only define MailBot-specific deltas.
- `git-workflow-guard` is the governance entry for commit branch/message/footer checks: `.opencode/skills/git-workflow-guard/SKILL.md`.

## Security & Configuration Tips
- `config.json` contains secrets; do not commit it. Use `config.example.json` as a template.
- Logs are written to `logs/`; sanitize data before sharing.

## PROGRESS Tracking and Skill Synchronization
- Use `.opencode/PROGRESS.md` as the progress index and governance file.
- Store each new record as a markdown file under `.opencode/progress/entries/YYYY/` using `YYYY-MM-DD-N.md` naming.
- Maintain a global TOC in `.opencode/PROGRESS.md` with date-based page IDs (`YYYYMMDD-N`) mapped to date, title, path, and keywords.
- Each entry must include the related commit ID.
- The `progress-tracker` skill lives at `.opencode/skills/progress-tracker/SKILL.md`.
