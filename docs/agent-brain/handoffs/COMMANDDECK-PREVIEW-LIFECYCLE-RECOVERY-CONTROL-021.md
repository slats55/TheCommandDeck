# COMMANDDECK-PREVIEW-LIFECYCLE-RECOVERY-CONTROL-021

## Task ID

`COMMANDDECK-PREVIEW-LIFECYCLE-RECOVERY-CONTROL-021`

## Branch

- Feature branch: `feature/commanddeck-preview-lifecycle-recovery-control-021`
- Base: `origin/main` at `96a539b99dc49db23167a3aa30786a9d4738d09f`
- Feature commit: `2a38e691`

## Objective

Add trusted preview lifecycle state and a non-destructive operator retirement control for stale/offline runtime-reported previews, while preserving provenance boundaries.

## Architecture Used

1. Added lifecycle metadata columns on `preview_registry`:
   - `retired_at`
   - `retired_by_type`
   - `retired_by_id`
2. Added workspace-scoped retirement query:
   - `RetirePreviewRegistryRecord`
3. Added active-list filtering:
   - `ListPreviewRegistryRecords` excludes retired entries (`retired_at IS NULL`)
4. Added authenticated API route:
   - `POST /api/commandrunner/previews/{previewId}/retire`
5. Added handler:
   - `HandleCommandDeckPreviewRetire`
   - derives actor from authenticated member context
   - preserves historical record (no deletion)
6. Added deterministic reactivation behavior:
   - trusted runtime upsert clears retirement fields on the same record
7. Added lifecycle status derivation in server response:
   - `registered | healthy | stale | offline | runtime_disconnected | retired`
8. Added UI action:
   - `Retire Preview` only for `stale|offline|runtime_disconnected`
   - calls trusted retire endpoint; refreshes preview list

## Files Changed

- `server/migrations/089_preview_registry_retirement_lifecycle.up.sql`
- `server/migrations/089_preview_registry_retirement_lifecycle.down.sql`
- `server/pkg/db/queries/preview_registry.sql`
- `server/pkg/db/generated/preview_registry.sql.go`
- `server/pkg/db/generated/models.go`
- `server/internal/handler/previewregistry.go`
- `server/internal/handler/previewregistry_test.go`
- `server/cmd/server/router.go`
- `packages/core/types/commanddeck.ts`
- `packages/core/types/index.ts`
- `packages/core/api/schemas.ts`
- `packages/core/api/client.ts`
- `apps/web/app/[workspaceSlug]/(dashboard)/commanddeck/page.tsx`
- `apps/web/app/[workspaceSlug]/(dashboard)/commanddeck/page.test.tsx`

## Security Boundaries

- Runtime provenance still server-derived from trusted daemon/runtime identity; no client runtime spoofing path introduced.
- Retirement is workspace/member scoped and does not permit cross-workspace mutation.
- `command_run_id` remains unlinked by retirement/lifecycle logic; no command provenance inference added.
- Existing self-hosted sync and read-only list endpoint behavior remain non-provenance-asserting.
- No arbitrary shell/process capability introduced.

## Tests and Verification Commands

### Git scope

- `git diff --check`
- `git diff --cached --stat`
- `git diff --cached --name-status`

### Backend

- `docker run --rm -v "${PWD}/server:/src" -w /src golang:1.26 /bin/sh -lc "export PATH=/usr/local/go/bin:$PATH; go test ./internal/daemon/cmdexec ./internal/daemonws ./internal/handler ./cmd/server && go build ./cmd/server"` (pass)
- Focused: `go test ./internal/handler -run Preview` in same Docker workflow (pass)

### Concurrency/race

- Attempted: `go test -race ./internal/daemonws ./internal/handler` in Docker
- Result: not executable in current container (`gcc` missing for CGO); recorded honestly.

### SQLC / migrations

- `docker run --rm -v "${PWD}/server:/src" -w /src sqlc/sqlc:1.27.0 generate` (pass)
- Disposable DB migration replay (`multica_sprint_021_mig`) on local Docker Postgres:
  - `migrate up` (pass through `089`)
  - `migrate down` (pass; rolls back through `001` per repository migrate implementation)
  - `migrate up` again (pass through `089`)

### Frontend/API

- Focused:
  - `pnpm --filter @multica/web exec vitest run "app/[workspaceSlug]/(dashboard)/commanddeck/page.test.tsx"` (pass)
  - `pnpm --filter @multica/core exec vitest run api/client.test.ts api/schema.test.ts` (pass)
- Full no-cache acceptance runs:
  - `pnpm exec turbo run lint --force` (pass)
  - `pnpm exec turbo run test --force` (pass)
  - `pnpm exec turbo run build --force` (pass)
- Standard full commands:
  - `pnpm lint` (pass)
  - `pnpm test` (pass)
  - `pnpm build` (pass)

### Health

- `http://localhost:8080/health` -> `200`
- `http://localhost:3000` -> `200`
- `http://localhost:3000/login` -> `200`

## Gate Verdict

`CODEX_AUTHORIZED_ACCEPTANCE_GATE: GO`

## Merge and Origin Confirmation

- Pending at handoff write time; to be finalized during merge step in this sprint.

## Known Limitations

- This slice intentionally does not add command-run provenance linkage.
- Lifecycle state is preview-ops focused and still depends on runtime heartbeat and preview health evidence quality.

## Next Recommended Task

`COMMANDDECK-APPROVED-PREVIEW-LAUNCH-PROVENANCE-022` (only if trusted server-issued correlation is implementable without inference); otherwise pivot to `COMMANDDECK-WORKFLOW-EXECUTION-RECORD-FOUNDATION-022A`.
