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
- Dependencies: `go.mod` and `go.sum`.
- Packaging and entrypoints: `cmd/package`, `cmd/mailbot`, and release bundle contents.

# Checks
- Workflow triggers and job steps still match repository paths and scripts.
- Go/tool version assumptions match `go.mod`; standalone builds use `CGO_ENABLED=0`.
- Dependencies required by CI jobs are still installed at expected steps.
- Artifact names and output paths are consistent with packaging scripts.
- Logging/output in CI remains readable and not silently broken.

# Deliverables
- CI impact summary for the change.
- Any required workflow updates.
- Verification notes for commands run or rationale when command execution is not possible.
