# Known Issues

This file tracks known local and operational issues that future agents should not mistake for new product failures.

## Local Repo Health

- `pnpm run doctor` may warn that `.env` contains the default `JWT_SECRET` placeholder.
- The placeholder warning is acceptable for local repo-doctor closeout only when hard failures are `0`.
- The placeholder warning is not acceptable for shared or production readiness.

## Tooling

- `gh` may not be installed locally.
- GitHub operations can still work through Git Credential Manager even when `gh auth status` is unavailable.
- `rg` may not be installed in some shells. Use PowerShell-native file discovery when needed.

## Existing Docs

- `docs/agent-brain/handoffs/` already contains Docker Hub publish handoffs.
- `docs/agent-brain/runbooks/DOCKER-RUNBOOK.md` already contains Docker deployment notes.
- Do not overwrite existing handoffs or runbooks without inspecting them first.

## Scope Watch

- Agent-brain work should remain documentation-only unless a task explicitly approves repo config or product changes.
- Do not introduce Python worker services, Java services, queues, databases, auth changes, command runner changes, Docker changes, migrations, or CI changes from this scaffold.
