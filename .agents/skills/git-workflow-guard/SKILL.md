---
name: git-workflow-guard
description: Enforce Angular commit messages, Git Flow branches, and agent-specific Co-Author signatures for all commits.
---

# Purpose
Provide one reusable governance source for commit message quality, branch strategy, and agent auto-commit metadata.

# When to Use
- Before creating any commit for code, docs, config, or automation output.
- Before creating branches for feature, release, or hotfix work.
- During pre-merge validation to confirm branch and commit history compliance.

# Scope
- Applies to all commits (human and agent-generated).
- Applies to all branch activities involving `main`, `develop`, `feature/*`, `release/*`, and `hotfix/*`.

# Rules
## 1) Commit Message (Angular / Conventional Commits)
- Header format: `type(scope): subject`.
- Allowed `type`: `feat|fix|docs|style|refactor|test|chore|build|ci|perf|revert`.
- `subject` must start with lowercase, use imperative mood, avoid trailing period, and should stay concise (recommended <= 72 chars).
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

Extension rule:
- Before allowing a new agent to auto-commit, add its fixed `agent -> Co-authored-by` mapping in this skill.

## 4) Pre-commit Checklist
- Is the current branch name compliant with Git Flow naming?
- Does the commit message match Angular format and rules?
- If this is an agent auto-commit, is the required `Co-authored-by` footer present and correct?

# Deliverables
- A clear compliance decision for the current commit request (`pass` or `fail`).
- If failed: a concrete non-compliance list and fix template for message/branch/footer.

Fix template:
1. Message: `type(scope): subject`
2. Branch: `feature/<name>` or `release/<version>` or `hotfix/<name-or-version>`
3. Footer (agent only): `Co-authored-by: <Agent Name> <agent@email>`
