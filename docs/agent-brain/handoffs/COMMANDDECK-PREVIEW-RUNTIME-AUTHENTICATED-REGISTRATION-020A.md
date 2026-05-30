# COMMANDDECK-PREVIEW-RUNTIME-AUTHENTICATED-REGISTRATION-020A

## Task ID

`COMMANDDECK-PREVIEW-RUNTIME-AUTHENTICATED-REGISTRATION-020A`

## Branch

- Feature branch: `feature/commanddeck-preview-runtime-authenticated-registration-020a`
- Base: `origin/main` at `3b5c973348e003b8c6d917144c09db5b01b48b23`
- Feature commit: `8127f4b6`

## Objective

Add a trusted preview registration path where runtime provenance is derived from authenticated daemon/runtime identity, while preserving `command_run_id` as unlinked (`NULL`) unless a separately trusted correlation source exists.

## Architecture Used

1. Added daemon route:
   - `POST /api/daemon/runtimes/{runtimeId}/previews/report`
2. Added handler:
   - `Handler.ReportRuntimePreview`
3. Trust boundary:
   - Requires daemon-token auth path (`mdt_`) only.
   - Requires route runtime to exist and belong to the caller workspace.
   - Requires runtime daemon identity to match authenticated daemon ID from context.
   - Derives persisted `runtime_id` from route/runtime lookup (not payload).
4. Preview target validation:
   - Reuses existing `validatePreviewTarget` and bounded/no-redirect health probe.
5. Provenance persistence:
   - Persists `runtime_id` from trusted server context.
   - Keeps `command_run_id` unset (`NULL`) in this flow.
6. Existing self-hosted sync behavior unchanged:
   - `POST /api/commandrunner/previews/self-hosted/sync` still does not infer provenance.

## Files Changed

- `server/internal/handler/previewregistry.go`
- `server/internal/handler/previewregistry_test.go`
- `server/cmd/server/router.go`
- `apps/web/app/[workspaceSlug]/(dashboard)/commanddeck/page.tsx`
- `apps/web/app/[workspaceSlug]/(dashboard)/commanddeck/page.test.tsx`

## Tests Added/Updated

Backend tests added:

- `TestReportRuntimePreview_AssignsTrustedRuntimeAndKeepsCommandRunUnlinked`
- `TestReportRuntimePreview_RejectsSpoofedRuntimeIDPayload`
- `TestReportRuntimePreview_RejectsDaemonRuntimeMismatch`
- `TestReportRuntimePreview_RejectsCrossWorkspaceDaemon`
- `TestReportRuntimePreview_RequiresDaemonTokenAuthPath`

Frontend tests updated:

- Updated unlinked preview assertion to truthful label.
- Added `renders verified runtime provenance and offline runtime truth`.

## Gate Evidence

### Git Scope

- `git diff --stat`
- `git diff --name-status`
- `git diff --check`

### Doctor

- `powershell -ExecutionPolicy Bypass -File scripts/doctor.ps1` passed (expected dirty working tree warning during feature development).

### Backend

- `docker run --rm -v "${PWD}/server:/src" -w /src golang:1.26 /bin/sh -lc "export PATH=/usr/local/go/bin:$PATH; go test ./internal/daemon/cmdexec ./internal/daemonws ./internal/handler ./cmd/server && go build ./cmd/server"` passed.
- Focused:
  - `docker run --rm -v "${PWD}/server:/src" -w /src golang:1.26 /bin/sh -lc "export PATH=/usr/local/go/bin:$PATH; go test ./internal/handler -run Preview"` passed.

### Frontend

- Focused:
  - `pnpm --filter @multica/web exec vitest run "app/[workspaceSlug]/(dashboard)/commanddeck/page.test.tsx"` passed.
- Full:
  - `pnpm lint` passed (warnings only; no lint errors).
  - `pnpm test` passed.
  - `pnpm build` passed.

### Local Health

- `http://localhost:8080/health` -> `200`
- `http://localhost:3000` -> `200`
- `http://localhost:3000/login` -> `200`

## SQLC / Migration Impact

- No migration changes.
- No SQL query file changes.
- No SQLC regeneration required for this slice.

## Security Boundaries Confirmed

- No browser/public endpoint for runtime provenance spoofing was introduced.
- No payload-provided `runtime_id` is trusted.
- Cross-workspace daemon attempts are rejected.
- Daemon/runtime mismatches are rejected.
- No `command_run_id` inference was introduced.
- Existing preview target trust/redirect protections remain in use.

## Gate Verdict

`CODEX_AUTHORIZED_ACCEPTANCE_GATE: GO` for slice 020A on feature branch.

## Merge/Origin Confirmation

- Pending mainline merge at time of this handoff update.

## Known Limitations

- This slice establishes trusted runtime provenance only.
- It does not establish command-run provenance for previews.

## Next Recommended Task

`COMMANDDECK-PREVIEW-LIFECYCLE-RECOVERY-CONTROL-021`:
add trusted preview lifecycle state/retirement behavior on top of runtime-authenticated preview registration.
