# Repository Guidelines

## Project Structure & Module Organization
- `main.py` is the CLI entry point; `app/bootstrap.py` is the composition root.
- `service/mailbot/` holds business orchestration (manager, chat handler, pipeline, policies, handlers).
- `core/` holds reusable framework modules: `core/chat`, `core/ai`, `core/email`, and `core/runtime`.
- `interface/` contains the Rich-based menu and configuration wizards (UI layer only).
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
- Tests live under `test/` and run with `pytest` (legacy `unittest` style is still present in some files).
- `python -m pytest test -v` runs the full regression suite.
- `python -m pytest test/core test/service -v` runs architecture-focused tests.
- `python -m unittest test.test_litellm_proxy` runs the proxy smoke test (requires valid `config.json` and API keys).
- `python -m pytest test/test_proxy_double_proxy.py -v` runs the socket-proxy regression test.
- `python test/smoketest_litellm.py` verifies PyInstaller bundle imports.

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
- Use `.progress/PROGRESS.md` as the canonical progress index.
- Store each new record under `.progress/entries/YYYY/` using `YYYY-MM-DD-N.md` naming.
- Maintain the Global TOC in `.progress/PROGRESS.md` with page IDs (`YYYYMMDD-N`) and append one row per entry.
- Use superpowers `progress-bootstrap` when `.progress/` structure is missing, then log milestones with superpowers `progress-tracker`.
