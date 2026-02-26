---
name: progress-tracker
description: Use when major changes or blockers must be recorded and the .opencode progress index plus related governance docs need synchronization.
---

# Purpose
Ensure every blocker or substantial change is documented with reflection so future work can reuse lessons.

# Superpowers Boundary
- Use superpowers process skills for implementation workflow.
- This skill only governs MailBot progress recording structure and traceability.

# Scope & Inputs
- `.opencode/PROGRESS.md`: progress index and governance rules.
- `.opencode/progress/entries/YYYY/*.md`: per-event records with commit traceability.
- `.opencode/AGENTS.md`: governance guidance that must stay synchronized with new practices.
- `.opencode/skills/` and impacted `SKILL.md` files.
- Git HEAD commit ID for traceability.

# Procedure
1. Clarify the event context, affected areas, and why the change matters.
2. Add a markdown record under `.opencode/progress/entries/YYYY/` using `YYYY-MM-DD-N.md` (`N` is next sequence for that date).
3. Update `.opencode/PROGRESS.md` Global TOC with `Page ID = YYYYMMDD-N`, plus date, title, path, and keywords.
4. Review `.opencode/AGENTS.md` and impacted skills for updates.
5. Capture reusable lessons to avoid repeated mistakes.
6. Report changed files and outcome.

# Deliverables
- One complete entry file in `.opencode/progress/entries/YYYY/` (with commit ID).
- Updated `.opencode/PROGRESS.md` TOC row for that entry.
- Any required `.opencode/AGENTS.md` / skill updates kept in sync.
- Concise lessons learned summary.
