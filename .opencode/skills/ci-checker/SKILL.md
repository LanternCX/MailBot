---
name: ci-checker
description: Use when changes may affect MailBot GitHub Actions, packaging, dependency setup, or release artifacts.
---

# Purpose
Check whether repository-specific CI and release workflow behavior stays valid after changes.

# Superpowers Boundary
- Superpowers handles generic verification discipline.
- This skill captures MailBot CI topology and release-path checks.

# Scope
- Workflows: `.github/workflows/*.yml`.
- Dependencies: `requirements.txt` and related setup commands.
- Packaging and entrypoints: `scripts/package.py`, `main.py`, hooks used by packaging.

# Checks
- Workflow triggers and job steps still match repository paths and scripts.
- Python/tool version assumptions remain compatible with project baseline.
- Dependencies required by CI jobs are still installed at expected steps.
- Artifact names and output paths are consistent with packaging scripts.
- Logging/output in CI remains readable and not silently broken.

# Deliverables
- CI impact summary for the change.
- Any required workflow updates.
- Verification notes for commands run or rationale when command execution is not possible.
