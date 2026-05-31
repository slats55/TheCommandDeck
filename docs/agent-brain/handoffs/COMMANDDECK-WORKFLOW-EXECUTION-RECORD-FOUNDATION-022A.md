# COMMANDDECK-WORKFLOW-EXECUTION-RECORD-FOUNDATION-022A

- Task ID: `COMMANDDECK-WORKFLOW-EXECUTION-RECORD-FOUNDATION-022A`
- Branch: `feature/commanddeck-workflow-execution-record-foundation-022a`
- Base commit: `6f8098ec8f9baf5e942b8fbce4067bb183335496`
- Feature commit: `5f93bc0a`
- Release track: `R0.3`

## Objective

Add a real workspace-scoped workflow execution record foundation that can carry lifecycle status and optionally link existing command-run evidence without weakening workspace boundaries.

## Architecture Used

- New table: `command_workflow_execution`
- sqlc query layer: create/list/get/update-status
- Workspace-scoped CommandDeck API handlers under `/api/commandrunner/workflows`
- Reused existing auth/workspace member checks and command-run lookup path
- Reused existing CommandDeck page for a narrow operator-facing workflow section

## Files Changed

- `server/migrations/090_command_workflow_execution_foundation.up.sql`
- `server/migrations/090_command_workflow_execution_foundation.down.sql`
- `server/pkg/db/queries/command_workflow_execution.sql`
- `server/pkg/db/generated/command_workflow_execution.sql.go`
- `server/pkg/db/generated/models.go`
- `server/internal/handler/commandworkflow.go`
- `server/internal/handler/commandworkflow_test.go`
- `server/cmd/server/router.go`
- `packages/core/types/commanddeck.ts`
- `packages/core/types/index.ts`
- `packages/core/api/schemas.ts`
- `packages/core/api/client.ts`
- `packages/core/api/client.test.ts`
- `apps/web/app/[workspaceSlug]/(dashboard)/commanddeck/page.tsx`
- `apps/web/app/[workspaceSlug]/(dashboard)/commanddeck/page.test.tsx`

## Delivered Behavior

- Create/list/get/update workflow execution records in workspace scope.
- Lifecycle statuses: `planned`, `running`, `needs_review`, `completed`, `failed`, `cancelled`.
- Optional command-run evidence link (`command_run_id`) with explicit workspace validation.
- Cross-workspace command-run association rejected.
- CommandDeck UI now renders workflow records, truthful empty/data states, creation form, and bounded lifecycle progression actions.

## Security Boundaries

- Workspace member gate required for all workflow endpoints.
- `command_run_id` accepted only when the referenced run belongs to the same workspace.
- No fake seeded workflow records.
- No preview provenance inference added in this slice.

## Tests and Verification Commands

- Focused backend:
  - `docker run --rm -v "${PWD}/server:/src" -w /src golang:1.26 /bin/sh -lc "export PATH=/usr/local/go/bin:$PATH; go test ./internal/handler -run CommandWorkflowExecution"`
- Backend gate:
  - `docker run --rm -v "${PWD}/server:/src" -w /src golang:1.26 /bin/sh -lc "export PATH=/usr/local/go/bin:$PATH; go test ./internal/daemon/cmdexec ./internal/daemonws ./internal/handler ./cmd/server; go build ./cmd/server"`
- SQLC:
  - `docker run --rm -v "${PWD}/server:/src" -w /src sqlc/sqlc:1.27.0 generate`
- Migration replay (disposable DB):
  - `docker exec commanddeck-commanddeck-db-1 psql -U multica -d postgres -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS multica_sprint_022a_mig;" -c "CREATE DATABASE multica_sprint_022a_mig;"`
  - `docker run --rm -e DATABASE_URL="postgres://multica:multica@host.docker.internal:5432/multica_sprint_022a_mig?sslmode=disable" -v "${PWD}/server:/src" -w /src golang:1.26 /bin/sh -lc "export PATH=/usr/local/go/bin:$PATH; go run ./cmd/migrate up; go run ./cmd/migrate down; go run ./cmd/migrate up"`
- Focused frontend/API:
  - `pnpm.cmd --filter @multica/core test -- api/client.test.ts`
  - `pnpm.cmd --filter @multica/web test -- "app/[workspaceSlug]/(dashboard)/commanddeck/page.test.tsx"`
- Full workspace gates:
  - `powershell -ExecutionPolicy Bypass -File scripts/doctor.ps1`
  - `pnpm.cmd lint`
  - `pnpm.cmd test`
  - `pnpm.cmd build`
- Local health:
  - `http://localhost:8080/health`
  - `http://localhost:3000`
  - `http://localhost:3000/login`

## Known Limitations

- This slice does not implement preview launch command provenance correlation.
- `COMMANDDECK-APPROVED-PREVIEW-LAUNCH-PROVENANCE-022` remains deferred pending a trusted server-issued preview operation correlation path.

## Next Recommended Task

- `COMMANDDECK-APPROVED-PREVIEW-LAUNCH-PROVENANCE-022B`
