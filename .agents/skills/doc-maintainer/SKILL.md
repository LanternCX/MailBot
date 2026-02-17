---
name: doc-maintainer
description: Keep documentation accurate and in sync across English and Chinese guides when code or UX changes.
---

# Purpose
Ensure user-facing docs reflect current behavior, config schema, CLI flows, and logging expectations.

# When to Use
- Features, flags, or defaults change.
- Menu/wizard steps or outputs change.
- Config keys, notifier options, proxy behavior, or logging defaults change.
- Release packaging or startup instructions change.

# Sources of Truth
- Root READMEs: [README.md](../../../README.md) and [README_ZH.md](../../../README_ZH.md).
- Guides: [docs/en](../../../docs/en) and [docs/zh](../../../docs/zh) for setup/configuration.
- Screens/preview assets: [docs/img](../../../docs/img).

# Update Process
1) Identify impact: list changed behaviors, new flags, renamed config keys, and UX text.
2) Edit English first; keep headings and ordering aligned between languages for easy diff.
3) Mirror updates to Chinese docs; keep terminology consistent (do not auto-translate code/CLI tokens).
4) Update examples: CLI snippets, config snippets, and sample outputs; ensure JSON samples are valid.
5) Note prerequisites (Python version, dependencies like Rich/PySocks) and environment-specific steps (macOS Gatekeeper, proxies).
6) Refresh screenshots/previews if UI text or layout changed; update image links.
7) Cross-check quickstart paths (executable vs source) and headless instructions.
8) After edits, run lint/link/error checks (where available) and ensure no known errors remain.

# Style Guidelines
- Markdown with clear headings; use bullet lists over long paragraphs.
- Keep commands copy-pastable; annotate risky steps briefly (e.g., "clears quarantine").
- Describe defaults explicitly (poll interval, retries, log level, proxy off by default).
- Keep comments/explanations in English where present; retain CLI tokens verbatim.
- Avoid embedding secrets; truncate tokens/IDs in examples.

# Validation Checklist
- Both languages updated and structurally aligned.
- Links resolve within the repo; images present and referenced with correct paths.
- Instructions tested or reasoned for target OS (macOS/Linux/Windows).
- Changelist summarized in release notes or README "What's new" if scope is user-visible.
- Post-edit checks clean; outstanding issues documented with owners.

# Deliverables
- Updated Markdown files and assets.
- Brief change log of doc updates and any gaps left for follow-up.
- Confirmation that post-edit checks are clean or annotated.
