# COMMANDDECK-MAINLINE-CONSOLIDATION-006

## Phase A — Main preview baseline landed

- Starting `origin/main`: `51074e1a850c1f040b91b3ae983d27da5f5857c0`
- Preview recovery branch: `origin/integration/commanddeck-preview-recovery-005`
- Preview recovery branch tip: `1d32ada8fd93ae8324b043fcfe648239e910b31a`
- Pre-merge rollback tag:
  - `checkpoint/main-before-preview-recovery-005` -> `51074e1a850c1f040b91b3ae983d27da5f5857c0`
- Merge into `main`:
  - `merge: restore verified CommandDeck local preview baseline`
  - merge commit: `8d1ac8b226104d452869cd96aaa83acfadb24c18`
- Post-merge known-good tag:
  - `checkpoint/main-preview-running-005` -> `8d1ac8b226104d452869cd96aaa83acfadb24c18`
- Push result:
  - `origin/main` now at `8d1ac8b226104d452869cd96aaa83acfadb24c18`
  - local/main sync check: `0 0`

## Phase B — Historical branch lineage decisions

Primary consolidation candidate: `origin/feature/commanddeck-command-ledger-001`

Branches proven ancestor-of-ledger and treated as superseded:

- `origin/feature/commanddeck-overnight-006r` -> INCLUDED_VIA_LEDGER
- `origin/feature/commanddeck-ui-run-control-006` -> INCLUDED_VIA_LEDGER
- `origin/feature/commanddeck-security-gate-001` -> INCLUDED_VIA_LEDGER
- `origin/feature/commanddeck-slice-001-git-status-runner` -> INCLUDED_VIA_LEDGER
- `origin/feature/commanddeck-slice-002-build-verify-next-command` -> INCLUDED_VIA_LEDGER
- `origin/feature/commanddeck-slice-003-branch-command-v2` -> INCLUDED_VIA_LEDGER
- `origin/feature/commanddeck-slice-004-rev-parse-head` -> INCLUDED_VIA_LEDGER
- `origin/feature/commanddeck-slice-005-git-diff-stat` -> INCLUDED_VIA_LEDGER

Branch not ancestor of ledger:

- `origin/feature/commanddeck-slice-003-branch-command` -> PRESERVED_HISTORY_NO_MERGE

Unique branch comparisons (evaluated, not merged in this slice):

- `origin/test/commanddeck-command-runner-tests-004`
  - large unique test delta in `server/internal/daemon/cmdexec/executor_test.go`
  - disposition: PRESERVED_HISTORY_NO_MERGE (needs targeted reconciliation pass)
- `origin/docs/commanddeck-template-runner-design-001`
  - unique design doc only
  - disposition: PRESERVED_HISTORY_NO_MERGE (can be imported in docs-focused pass)

## Consolidation branch

- Branch: `integration/commanddeck-command-runner-ledger-006`
- Base `main` commit: `8d1ac8b226104d452869cd96aaa83acfadb24c18`
- Ledger merge strategy: `--no-ff --no-commit`, then scope cleanup and verification before commit

## Key integration/repair work applied

1. Integrated command-runner/ledger backend + UI + schema layer from ledger branch:
   - CommandDeck page route
   - command runner API handlers/routes
   - daemon command execution bridge
   - migrations `084`, `085`, `086`
   - SQL queries + generated db accessors

2. Kept local preview baseline/auth branding intact while merging.

3. Added compatibility test updates required by new CommandDeck route key:
   - `packages/core/paths/consistency.test.ts`
   - `packages/views/layout/app-sidebar.test.tsx`

4. Fixed runtime-breaking migration defect discovered during preview bring-up:
   - root cause:
     - `084_command_template.up.sql` seeded templates with workspace UUID `00000000-...`
     - FK `command_template.workspace_id -> workspace.id` rejected insert
     - API container crash-looped on migration
   - fix:
     - make `command_template.workspace_id` nullable
     - seed built-in templates with `workspace_id = NULL`
     - update template lookup/list queries to support global built-ins with workspace override precedence
     - update handler workspace validation to allow global built-ins
   - result:
     - migrations `084`, `085`, `086` now apply successfully
     - API starts healthy

## Files changed in this slice

Includes code/config only (legacy handoff docs from historical branch were intentionally excluded from this merge commit scope):

- `apps/web/app/[workspaceSlug]/(dashboard)/commanddeck/page.tsx`
- `packages/core/api/client.ts`
- `packages/core/paths/paths.ts`
- `packages/core/paths/consistency.test.ts`
- `packages/core/types/commanddeck.ts`
- `packages/core/types/index.ts`
- `packages/views/layout/app-sidebar.tsx`
- `packages/views/layout/app-sidebar.test.tsx`
- `packages/views/locales/en/layout.json`
- `packages/views/locales/zh-Hans/layout.json`
- `server/cmd/server/router.go`
- `server/internal/daemon/cmdexec/daemon.go`
- `server/internal/daemon/cmdexec/executor.go`
- `server/internal/daemon/cmdexec/executor_test.go`
- `server/internal/daemon/daemon.go`
- `server/internal/daemon/wakeup.go`
- `server/internal/daemonws/hub.go`
- `server/internal/handler/commandrunner.go`
- `server/internal/middleware/daemon_auth.go`
- `server/internal/middleware/daemon_auth_test.go`
- `server/migrations/084_command_template.down.sql`
- `server/migrations/084_command_template.up.sql`
- `server/migrations/085_command_run.down.sql`
- `server/migrations/085_command_run.up.sql`
- `server/migrations/086_command_ledger.down.sql`
- `server/migrations/086_command_ledger.up.sql`
- `server/pkg/db/queries/command_template.sql`
- `server/pkg/db/queries/command_run.sql`
- `server/pkg/db/queries/command_ledger.sql`
- `server/pkg/db/generated/command_template.sql.go`
- `server/pkg/db/generated/command_run.sql.go`
- `server/pkg/db/generated/command_ledger.sql.go`
- other `server/pkg/db/generated/*.sql.go`, `db.go`, `models.go` updates from ledger merge
- `server/pkg/protocol/command_run.go`

## Verification executed

### Static/build/test

- `powershell -ExecutionPolicy Bypass -File scripts/doctor.ps1` -> PASS (hard failures: 0)
- `pnpm run doctor:ps` -> FAIL in this environment (`pwsh` missing)
- `pnpm lint` -> PASS (pre-existing warnings only)
- `pnpm build` -> PASS
- `pnpm test` -> PASS
- `pnpm --filter @multica/web exec vitest run "app/(auth)/login/page.test.tsx"` -> PASS
- `pnpm --filter @multica/views exec vitest run auth/login-page.test.tsx` -> PASS
- `go test ./...` from `server/` -> FAIL (`go` not installed on host PATH)
- `go build ./...` from `server/` -> FAIL (`go` not installed on host PATH)

### Preview/runtime

- `docker compose -f compose.yml -f compose.dev.yml up -d --build` -> PASS
- `docker compose -f compose.yml -f compose.dev.yml ps` -> PASS (`commanddeck-api`, `commanddeck-web`, `commanddeck-db`, `commanddeck-redis` up)
- `http://localhost:8080/health` -> `200`, body `{"status":"ok"}`
- `http://localhost:3000` -> `200`
- `http://localhost:3000/login` -> `200`
  - contains `Sign in to CommandDeck`
  - does not contain `Sign in to Multica`

### Command runner endpoint sanity

Using authenticated API flow and a real workspace slug created for local verification:

- workspace slug used: `commanddeck-local`
- `GET /api/commandrunner/templates?workspace_slug=commanddeck-local` -> 4 templates
- `GET /api/commandrunner/runs?workspace_slug=commanddeck-local` -> 0 runs initially

### CommandDeck UI route check

- `GET http://localhost:3000/commanddeck-local/commanddeck` (unauthenticated HTTP probe) resolves but presents login.
- In this CLI-only verification path, authenticated browser-session rendering of the page could not be asserted without interactive browser sign-in/session handoff.
- Build output confirms route registration: `/[workspaceSlug]/commanddeck`.

## Security and scope notes

- No secrets committed.
- `.env` not committed.
- No force-push.
- No arbitrary shell execution feature added.
- Execution path remains allowlisted/template-bound (`git status`, `git branch`, `git rev-parse`, `git diff`).
- Workspace boundary checks remain in executor/handler flow.

## Proposed next merge action

Run a focused gate/merge task for:

- `integration/commanddeck-command-runner-ledger-006` -> `main`

with emphasis on:

1. final review of command execution safety boundaries,
2. confirming whether to import unique tests from `test/commanddeck-command-runner-tests-004`,
3. optional interactive browser validation of authenticated CommandDeck page render.
