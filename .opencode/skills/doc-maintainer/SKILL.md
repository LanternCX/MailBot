---
name: doc-maintainer
description: Use when MailBot behavior/config/UX changes and both English and Chinese documentation must remain synchronized.
---

# Purpose
Keep user-facing docs accurate and aligned across English and Chinese guides.

# Superpowers Boundary
- Superpowers handles generic planning and execution discipline.
- This skill defines MailBot-specific documentation scope and bilingual sync rules.

# Sources of Truth
- Root READMEs: [README.md](../../../README.md) and [README_ZH.md](../../../README_ZH.md).
- Guides: [docs/en](../../../docs/en), [docs/zh](../../../docs/zh), [docs/dev](../../../docs/dev).
- Assets: [docs/img](../../../docs/img).

# Update Process
1. Identify impacted behavior, config keys, flags, and UX text.
2. Update English first; keep heading structure stable.
3. Mirror updates to Chinese docs with consistent terminology.
4. Refresh command/config examples and keep JSON valid.
5. Re-check platform-specific instructions (macOS/Linux/Windows) and prerequisites.
6. Refresh image references when UI text or layout changes.

# Validation Checklist
- English and Chinese docs are structurally aligned.
- Internal links and image paths resolve.
- Quickstart, setup, and headless instructions remain executable.
- Sensitive values are not exposed in examples.

# Deliverables
- Updated Markdown files and assets.
- Short changelog of doc updates and any deferred follow-ups.
