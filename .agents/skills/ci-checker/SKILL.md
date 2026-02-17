---
name: ci-checker
description: Project release checker to assess CI/GitHub Actions impact before merge or deployment.
---

# Purpose
Verify that code/docs/config changes will not break CI pipelines (GitHub Actions) and that workflows stay green after release.

# When to Use
- Before merging PRs or tagging releases.
- When modifying dependencies, Python version, workflow triggers, or logging defaults that affect tests.
- When altering scripts invoked by workflows (tests, packaging, lint).

# Scope & Inputs
- Workflow files: .github/workflows/*.yml.
- Runtime matrix: Python versions, OS targets.
- Project scripts: setup, tests, packaging (e.g., scripts/package.py, main.py entrypoints).
- Dependency manifests: requirements.txt, pinned tools.

# Checks
- Workflow integrity: YAML valid; triggers intentional; required secrets documented.
- Job steps: commands still exist and match project structure; paths/files referenced are present.
- Python/tool versions: remain supported; adjust matrix if baseline changes.
- Dependencies: new packages installed where workflows expect them; cache keys updated if dependency files change.
- Logging changes: ensure CI log level/format remains readable and not excessively noisy or silent.
- Artifacts: names/paths consistent with release packaging scripts.
- External effects: rate limits or API changes called in CI are guarded or mocked.

# Procedure
1) Review diffs touching workflows, dependencies, packaging, logging, or test commands.
2) Cross-check workflow steps against repo paths and scripts.
3) Run local equivalents when possible (lint/tests/package) or reason through expected output.
4) If workflow needs changes, update .github/workflows with minimal, documented edits.
5) Summarize risk and required follow-ups.
6) Before concluding, run error checks; do not leave known CI-impacting errors.

# Deliverables
- Findings on CI impact and any workflow edits applied.
- Confirmed commands/tests or rationale when not run.
- Confirmation that post-check error scans are clean; remaining risks listed with owners.
