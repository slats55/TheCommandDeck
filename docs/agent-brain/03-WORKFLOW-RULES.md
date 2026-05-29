# Workflow Rules

CommandDeck tasks must be evidence-based and scoped.

## Task Rules

- Every task gets a task ID.
- Every task starts by confirming repo root, branch, and dirty tree state.
- Every task returns a closure report.
- Verification evidence must be listed in the closure report.
- Blockers must be explicit.
- Do not hide skipped verification.
- Do not start the next slice until the current slice is closed or explicitly paused.

## Git Hygiene

- Always confirm repo root with `git rev-parse --show-toplevel`.
- Always confirm current branch with `git branch --show-current`.
- Always check dirty tree with `git status --short`.
- Always fetch before branch work with `git fetch origin --prune`.
- Use `git pull --ff-only origin main` when updating `main`.
- Do not merge feature branches into `main` from local automation.
- Do not use `git reset --hard` unless Myles explicitly authorizes it.
- Do not discard local work.

## Reporting Rules

- Never claim a push succeeded without push output or follow-up sync evidence.
- Never claim a branch is synced without an ahead/behind count.
- Never claim a runtime is healthy without a command result.
- Never claim completion when checks were not run.
- Use final statuses that distinguish complete, blocked, failed, and not verified.

## Handoff Rules

- Builder handoffs should identify changed files, command evidence, and known risks.
- Verifier handoffs should identify independent checks and residual risk.
- Gatekeeper handoffs should state merge readiness and blockers.
- Handoffs belong in `docs/agent-brain/handoffs/`.

## Scope Rules

- Documentation slices should stay documentation-only.
- Product code changes require explicit task approval.
- Auth, command execution, Docker, migrations, CI, and deployment changes require explicit scope.
- New architecture layers require repo evidence and Myles approval.
