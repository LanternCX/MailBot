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

## Security & Configuration Tips
- `config.json` contains secrets; don’t commit it. Use `config.example.json` as a template.
- Logs are written to `logs/`; sanitize data before sharing.
