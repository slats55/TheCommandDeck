# Current State

This file captures the baseline known at the start of `COMMANDDECK-AGENT-BRAIN-SCAFFOLD-002`.

## Repository

- Repo root: `C:/Users/mtval/TheCommandDeck`
- Remote: `origin` at `https://github.com/slats55/TheCommandDeck.git`
- Main branch: `main`
- Latest synced `main` baseline: `289be00781bfe922f4babacddff86d5b9736aa2f`
- Package manager: `pnpm`
- Repo health command: `pnpm run doctor`

## Architecture Facts

- The repo is still structurally a forked Multica monorepo.
- Frontend/product areas live under `apps/` and `packages/`.
- Backend/runtime areas live under `server/` and Docker-related files.
- Existing CommandDeck operational docs live under `docs/agent-brain/`.
- Existing agent-brain content before this scaffold included Docker handoffs and a Docker runbook.

## Health Baseline

- GitHub read and push access works through Git Credential Manager.
- Git author identity is configured repo-local.
- `pnpm run doctor` is the standard local repo health check.
- The `.env` JWT placeholder warning may appear during local checks. It is acceptable for local closeout when the doctor reports zero hard failures, but it must not be ignored for shared or production readiness.

## Current Intent

CommandDeck should use a TypeScript plus Python ownership model unless existing repo evidence requires otherwise:

- TypeScript owns dashboard, product surfaces, control-plane contracts, UI state, workflow state, approval gates, API clients, and realtime display.
- Python is reserved for AI planning, prompt generation, repo audits, fake-data scans, verification reports, handoff validation, log summarization, and worker tools.

Do not introduce Python services, Java services, new queues, new databases, or new architecture layers without explicit task scope and evidence.
