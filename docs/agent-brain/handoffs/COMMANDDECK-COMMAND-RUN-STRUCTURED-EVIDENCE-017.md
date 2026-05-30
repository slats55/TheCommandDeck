# COMMANDDECK-COMMAND-RUN-STRUCTURED-EVIDENCE-017

## Task ID
`COMMANDDECK-CONTROL-PLANE-COMMERCIAL-SPRINT-017`

## Branch
`feature/commanddeck-command-run-structured-evidence-017`

## Base
`origin/main` at `d8fcee768ea86c10e068e19df6b9669d05b647e4` (safe-cancellation merged)

## Why This Branch
Safe cancellation (`016`) passed focused acceptance gate and was merged. This slice adds durable structured safety evidence for truncation/cancellation.

## Implementation Summary

### Structured evidence persistence
- Added migration `088_command_run_structured_safety_evidence`:
  - `stdout_truncated BOOLEAN NOT NULL DEFAULT FALSE`
  - `stderr_truncated BOOLEAN NOT NULL DEFAULT FALSE`
  - `cancellation_requested_at TIMESTAMPTZ`
  - `cancellation_requested_by_type TEXT` (`member|agent`)
  - `cancellation_requested_by_id UUID`

### Query/API model updates
- `server/pkg/db/queries/command_run.sql`
  - `UpdateCommandRunResult` now persists `stdout_truncated` / `stderr_truncated`.
  - new `MarkCommandRunCancellationRequested`.
- Regenerated sqlc artifacts (`command_run.sql.go`, `models.go`).

### Daemon/executor evidence wiring
- `Executor.Result` now includes `StdoutTruncated` and `StderrTruncated`.
- Daemon command result payload now carries truncation booleans.

### Handler evidence wiring
- Cancellation handler now persists cancellation-request metadata (`requested_at`, requester type/id) before dispatching cancel frame.
- Command-run result handler now persists truncation booleans.
- Command-run response now returns:
  - `stdout_truncated`
  - `stderr_truncated`
  - `cancellation_requested_at`
  - `cancellation_requested_by_type`
  - `cancellation_requested_by_id`

### Frontend/API contract
- `packages/core/types/commanddeck.ts` updated with structured evidence fields.
- CommandDeck page now renders:
  - `truncated` indicator when stdout/stderr truncation flags are set.
  - `cancel requested <time>` when cancellation request timestamp exists.
- Updated CommandDeck page tests for structured evidence rendering.

### Roadmap artifact
- Updated `docs/commanddeck/02-ROADMAP.md` to a durable commercial delivery roadmap with:
  - mission
  - R0.1-R0.5 release tracks
  - dependency graph
  - slice registry
  - explicit commercial-readiness definition

## Files Changed
- `server/migrations/088_command_run_structured_safety_evidence.up.sql`
- `server/migrations/088_command_run_structured_safety_evidence.down.sql`
- `server/pkg/db/queries/command_run.sql`
- `server/pkg/db/generated/command_run.sql.go`
- `server/pkg/db/generated/models.go`
- `server/pkg/protocol/command_run.go`
- `server/internal/daemon/cmdexec/executor.go`
- `server/internal/daemon/cmdexec/daemon.go`
- `server/internal/handler/commandrunner.go`
- `packages/core/types/commanddeck.ts`
- `apps/web/app/[workspaceSlug]/(dashboard)/commanddeck/page.tsx`
- `apps/web/app/[workspaceSlug]/(dashboard)/commanddeck/page.test.tsx`
- `docs/commanddeck/02-ROADMAP.md`

## Verification Executed
- `powershell -ExecutionPolicy Bypass -File scripts/doctor.ps1` (pass)
- `docker run ... go test ./internal/daemon/cmdexec ./internal/handler ./cmd/server && go build ./cmd/server` (pass)
- `pnpm --filter @multica/core test -- api/client.test.ts` (pass)
- `pnpm --filter @multica/web test -- "app/[workspaceSlug]/(dashboard)/commanddeck/page.test.tsx"` (pass)
- `pnpm lint` (pass, existing warnings)
- `pnpm build` (pass)
- `pnpm test` (fails in pre-existing/flaky `@multica/views` tests; same class previously reported)
- Health probes:
  - `http://localhost:8080/health` => 200
  - `http://localhost:3000` => 200
  - `http://localhost:3000/login` => 200

## Security Notes
- No PID-based cancellation path introduced.
- No raw shell/process control exposure introduced.
- Command allowlist unchanged.
- Workspace scoping/auth checks remain on cancellation route.
- Structured evidence uses server-owned run identity and truthful persisted timestamps/flags.

## Known Risks
- `pnpm test` full monorepo reliability remains unstable in `@multica/views` under this environment; this is tracked as delivery-gate reliability risk and should be addressed in a focused follow-up if required by gate policy.

## Independent Gate Steps
1. `git fetch origin`
2. `git checkout feature/commanddeck-command-run-structured-evidence-017`
3. Verify scope:
   - migration 088
   - command_run query/generated changes
   - handler/daemon/protocol wiring
   - CommandDeck UI/type updates
   - roadmap update
4. Rerun:
   - doctor
   - backend docker go test/build
   - focused CommandDeck web/core tests
   - lint/build
   - full `pnpm test` (record reliability classification)

## One Recommended Next Slice
`fix/commanddeck-test-gate-reliability-017` if gate policy requires deterministic full-suite green before merging feature slices.
