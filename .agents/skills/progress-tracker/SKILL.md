---
name: progress-tracker
description: Track blocking issues and major changes, then update PROGRESS.md plus AGENTS/skills documentation.
---

# Purpose
Ensure every blocker or substantial change is documented with reflection so the team learns and AGENTS/skills stay aligned with actual practices.

# When to Use
- Encountering blockers, bugs, architectural drift, or behavior changes that affect multiple areas.
- Executing work that touches multiple modules, docs, or workflows.
- After introducing or revising any skill, tracking sheet, or governance instruction that requires confirmation steps.

# Scope & Inputs
- `.agents/PROGRESS.md`: progress index and governance rules.
- `.agents/progress/entries/YYYY/*.md`: per-event records with commit traceability.
- `.agents/AGENTS.md`: governance guidance that must stay synchronized with new practices.
- `.agents/skills/` directory and its `SKILL.md` files: update each skill that is impacted by the change.
- Git HEAD commit ID: cite the relevant ID before wrapping up the change.

# Procedure
1. Clarify the event: describe context, affected areas, and why the change matters.
2. Add a new markdown record under `.agents/progress/entries/YYYY/` using filename `YYYY-MM-DD-N.md` where `N` is the next sequence on that date.
3. Update `.agents/PROGRESS.md` Global TOC with `Page ID = YYYYMMDD-N`, plus date, title, path, and keywords.
4. Review AGENTS and impacted skills for necessary updates; apply edits to keep the documentation accurate.
5. Capture lessons learned so future contributors can reference this entry instead of repeating the same mistake.
6. Report the completed steps and changed files when providing feedback.

# Deliverables
- One complete entry file in `.agents/progress/entries/YYYY/` (with commit ID).
- Updated `.agents/PROGRESS.md` TOC that maps a unique date-based page ID to the new entry.
- Updated AGENTS.md and any touched skill documentation that reflect the new behavior.
- A concise summary of lessons learned for future reference.
