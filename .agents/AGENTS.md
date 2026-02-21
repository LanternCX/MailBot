# Repository Guidelines

## Project Structure & Module Organization
- `main.py` is the CLI entry point and service launcher.
- `core/` holds the runtime engine: models, manager, fetcher, parser, AI logic, bot handlers, rules, and `notifiers/`.
- `interface/` contains the Rich-based menu and configuration wizards.
- `utils/` provides shared helpers and logging (`utils/logger.py`).
- `docs/` includes setup and configuration guides; images live under `docs/img/`.
- `test/` contains smoke and integration tests.
- `scripts/` includes release tooling like `scripts/package.py`.
- `hooks/` stores build hooks; `logs/` is generated at runtime.

## Build, Test, and Development Commands
- `python -m venv .venv && source .venv/bin/activate`
- `pip install -r requirements.txt`
- `python main.py` runs MailBot from source.
- `python scripts/package.py --clean --variant macos-arm64` builds a PyInstaller bundle (requires PyInstaller installed).

## Coding Style & Naming Conventions
- Target Python 3.10+ with `from __future__ import annotations` in modules.
- Use type hints and concise docstrings; keep functions single-purpose.
- Use module loggers like `logging.getLogger("mailbot.<module>")`; avoid `print` except CLI prompts.
- Keep user-facing text in English and avoid logging secrets.

## Testing Guidelines
- Tests live under `test/` and use `unittest` (some commands use `pytest`).
- `python -m unittest test.test_litellm_proxy` runs the proxy smoke test (requires valid `config.json` and API keys).
- `python -m pytest test/test_proxy_double_proxy.py -v` runs the socket-proxy regression test.
- `python test/smoketest_litellm.py` verifies PyInstaller bundle imports.

## Commit & Pull Request Guidelines
- Follow Conventional Commits as seen in history: `feat:`, `fix:`, `docs:`.
- PRs should include a brief summary, tests run, and any config or docs changes.
- If you touch user-facing behavior, update `docs/en/` (and `README_ZH.md` when applicable).

## Git Workflow Governance
- `git-workflow-guard` skill is the default governance entry for all commit-related checks: `/Users/caoxin/Code/MailBot/.agents/skills/git-workflow-guard/SKILL.md`.
- All commits (human and agent) must follow Angular/Conventional Commit formatting and Git Flow branch naming.
- Agent auto-commits must include the mapped agent-specific `Co-authored-by` footer defined in the skill.

## Security & Configuration Tips
- `config.json` contains secrets; don’t commit it. Use `config.example.json` as a template.
- Logs are written to `logs/`; sanitize data before sharing.

## PROGRESS Tracking and Skill Synchronization
- Use `.agents/PROGRESS.md` as the progress index and governance file.
- Store each new record as an individual markdown file under `.agents/progress/entries/YYYY/` using `YYYY-MM-DD-N.md` naming.
- Maintain a global TOC in `.agents/PROGRESS.md` with date-based page IDs (`YYYYMMDD-N`) mapped to date, title, path, and keywords.
- Each entry must include the related commit ID. After recording, evaluate whether `.agents/AGENTS.md` or any affected `.agents/skills/*/SKILL.md` files require updates to match the new workflow or skill behavior.
- The `progress-tracker` skill lives at `.agents/skills/progress-tracker/SKILL.md`. When using this workflow, create/update both the entry file and TOC row, then sync AGENTS/skills documentation when process behavior changes.
