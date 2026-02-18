---
name: skill-manager
description: Align and evolve skills with current project scope, architecture standards, and delivery milestones.
---

# Purpose
Continuously curate the skills library so it matches active workstreams, release goals, logging standards, and maintainability requirements.

# When to Use
- Before/after sprints or release milestones to ensure skills reflect new priorities.
- When a task does not map cleanly to an existing skill.
- When existing skills are outdated, overlapping, or missing guardrails.

# Responsibilities
- Audit skill coverage against roadmap, open PRs, and recent incidents.
- Add, update, or deprecate skills to keep guidance relevant and non-conflicting.
- Ensure each skill has clear purpose, triggers, checklists, and deliverables.
- Keep references (paths/links) valid as docs/code move.
- Enforce architecture coherence (high cohesion / low coupling) and logging expectations across skills.
- Require post-response error checks until clean before closing a task.

# Operating Procedure
1) Assess scope: list active features/bugs and planned releases.
2) Map tasks → skills; note gaps or conflicts.
3) If gaps: draft or update skill with concise rules, examples, and checklists.
4) Validate consistency: avoid duplicated guidance; prefer single source of truth per area.
5) Communicate changes: surface updated skills to contributors and link from onboarding notes.
6) Re-audit periodically (e.g., per release) to retire stale skills.

# Quality Bar
- English comments and instructions; concise and actionable.
- Link to authoritative files (README, docs, configs) using relative paths.
- Keep style aligned with project norms (PEP 8, typing, logging conventions).
- Confirm no analyzer/runtime errors remain after updates.

# Deliverables
- New or updated SKILL files committed under .agents/skills/.
- A brief change log describing skill adjustments and rationale.
- Confirmation that post-update error checks are clean.
