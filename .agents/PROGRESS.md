# PROGRESS Table of Contents

## Purpose
- Keep long-term operational learning inside `.agents/` with a toolbook-style index.
- Store each record in a dedicated entry file and use this file as the global lookup catalog.

## Storage Layout
- TOC file: `.agents/PROGRESS.md`
- Entry root: `.agents/progress/entries/`
- Year folder pattern: `.agents/progress/entries/YYYY/`
- Entry filename pattern: `YYYY-MM-DD-N.md` (`N` starts from `1` per day)
- Page ID pattern: `YYYYMMDD-N`

## Entry Template
1. **Date**: YYYY-MM-DD
2. **Title**: Short actionable title
3. **Background / Issue**: Brief context and trigger
4. **Actions / Outcome**: What changed and what result was achieved
5. **Lessons / Refinements**: Reusable takeaways and prevention rules
6. **Related commit**: Commit ID for traceability

## TOC Rules
- Always append a TOC row for every new entry file.
- Never create an entry without TOC registration.
- Sort rows by `Date` ascending; same-day rows sorted by `N` ascending.

## Global TOC
| Page ID | Date | Title | Path | Keywords |
| --- | --- | --- | --- | --- |
| 20260221-1 | 2026-02-21 | Initialized PROGRESS workflow and progress-tracker skill | `./progress/entries/2026/2026-02-21-1.md` | progress-tracker, governance, logging |
| 20260221-2 | 2026-02-21 | Added git-workflow-guard governance skill | `./progress/entries/2026/2026-02-21-2.md` | git-flow, conventional-commits, co-author |
| 20260222-1 | 2026-02-22 | Migrated PROGRESS to .agents structured entries | `./progress/entries/2026/2026-02-22-1.md` | toc, structure, indexing |
