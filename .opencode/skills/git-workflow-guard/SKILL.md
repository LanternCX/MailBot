---
name: git-workflow-guard
description: Use when creating branches or commits in this repository and you must enforce MailBot git-flow, commit format, and agent co-author footer rules.
---

# Purpose
Provide one project-specific governance source for commit quality, branch strategy, and required agent co-author metadata.

# Superpowers Boundary
- Use superpowers for generic implementation and verification workflows.
- This skill only defines MailBot-specific git governance rules.

# When to Use
- Before creating any commit for code, docs, config, or automation output.
- Before creating branches for feature, release, or hotfix work.
- During pre-merge validation to confirm branch and commit history compliance.

# Scope
- Applies to all commits (human and agent-generated).
- Applies to branch activities involving `main`, `develop`, `feature/*`, `release/*`, and `hotfix/*`.

# Rules
## 1) Commit Message (Angular / Conventional Commits)
- Header format: `type(scope): subject`.
- Allowed `type`: `feat|fix|docs|style|refactor|test|chore|build|ci|perf|revert`.
- `subject` starts with lowercase, uses imperative mood, avoids trailing period, and should stay concise (recommended <= 72 chars).
- Breaking changes must include `BREAKING CHANGE: ...` in body or footer.

Examples:
- Valid: `feat(mail): add retry policy for smtp timeout`
- Invalid: `Update code`
- Invalid: `fix: bug.`

## 2) Branching (Git Flow)
- Long-lived branches: `main`, `develop`.
- Feature branch naming: `feature/<name>`.
- Release branch naming: `release/<version>`.
- Hotfix branch naming: `hotfix/<name-or-version>`.
- Do not do regular development directly on `main`; create `feature/*` from `develop`.

Examples:
- Valid: `feature/email-parser-refactor`, `release/1.4.0`, `hotfix/smtp-crash`
- Invalid: `bugfix/x`, `dev2`, direct feature development on `main`

## 3) Agent Auto-commit Co-Author (Mandatory)
Agent-generated commits must include a matching footer from the fixed mapping below:

- `codex` -> `Co-authored-by: Codex <codex@openai.com>`
- `copilot` -> `Co-authored-by: GitHub Copilot <copilot@github.com>`
- `opencode` -> `Co-authored-by: opencode-agent[bot] <opencode-agent[bot]@users.noreply.github.com>`

Extension rule:
- Before allowing a new agent to auto-commit, add its fixed `agent -> Co-authored-by` mapping in this skill.

## 4) Pre-commit Checklist
- Branch name compliant with Git Flow naming.
- Commit message matches Angular format and rules.
- Agent auto-commit contains required `Co-authored-by` footer.

# Deliverables
- Compliance decision for current commit request (`pass` or `fail`).
- If failed: concrete non-compliance list and a fix template for message/branch/footer.

Fix template:
1. Message: `type(scope): subject`
2. Branch: `feature/<name>` or `release/<version>` or `hotfix/<name-or-version>`
3. Footer (agent only): `Co-authored-by: <Agent Name> <agent@email>`
