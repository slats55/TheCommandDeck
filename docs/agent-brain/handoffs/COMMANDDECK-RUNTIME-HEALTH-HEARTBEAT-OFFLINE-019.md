# COMMANDDECK-RUNTIME-HEALTH-HEARTBEAT-OFFLINE-019

## Task ID
`COMMANDDECK-RUNTIME-HEALTH-HEARTBEAT-OFFLINE-019`

## Branch
`feature/commanddeck-runtime-health-heartbeat-offline-019`

## Base
`origin/main` at `45bf5b820df745306cb8cca9be2385bee4a4519e` (live command-run events `018` merged)

## Objective
Deliver truthful runtime heartbeat/offline visibility in CommandDeck using trusted daemon heartbeat evidence and workspace-scoped runtime data, without introducing fake runtime signals.

## Implementation Summary

### Backend: server-derived health policy
- Extended runtime response shape to include:
  - `health_status` (`online | stale | offline | unknown`)
  - `heartbeat_age_seconds` (nullable)
- Added centralized server derivation policy in `runtime.go`:
  - `unknown` when `last_seen_at` is missing
  - `online` when runtime is online and heartbeat age <= 45s
  - `stale` when runtime is online with old heartbeat, or recently offline
  - `offline` when runtime has been offline beyond the recent window
- Added focused unit tests for derivation boundaries and edge cases:
  - missing `last_seen_at`
  - online fresh vs old
  - offline recent vs old
  - future timestamp clamping

### Frontend: CommandDeck runtime health board
- Added runtime health metadata fields to core runtime type contract.
- Added a new **Runtime Health** panel on CommandDeck page:
  - loading state
  - empty state
  - per-runtime rows with provider/mode
  - health badge using server-derived `health_status`
  - last-seen evidence timestamp (or unknown)
- Kept run execution guard intact:
  - runtime picker remains restricted to `status === "online"`
  - explicit message when no online runtime is available

## Files Changed
- `server/internal/handler/runtime.go`
- `server/internal/handler/runtime_health_test.go`
- `packages/core/types/agent.ts`
- `apps/web/app/[workspaceSlug]/(dashboard)/commanddeck/page.tsx`
- `apps/web/app/[workspaceSlug]/(dashboard)/commanddeck/page.test.tsx`
- `docs/commanddeck/02-ROADMAP.md`

## Verification Executed
- Focused frontend:
  - `pnpm --filter @multica/web test -- "app/[workspaceSlug]/(dashboard)/commanddeck/page.test.tsx"`
- Backend:
  - `docker run --rm -v "${PWD}/server:/src" -w /src golang:1.26 /bin/sh -lc "export PATH=/usr/local/go/bin:$PATH; go test ./internal/daemon/cmdexec ./internal/daemonws ./internal/handler ./cmd/server && go build ./cmd/server"`
- Gate checks:
  - `powershell -ExecutionPolicy Bypass -File scripts/doctor.ps1`
  - `pnpm lint`
  - `pnpm test`
  - `pnpm build`
- Local health probes:
  - `http://localhost:8080/health` -> 200
  - `http://localhost:3000` -> 200
  - `http://localhost:3000/login` -> 200

## Security / Truth Notes
- Heartbeat writes remain on existing trusted daemon auth paths (`/api/daemon/heartbeat`, daemon WS heartbeat).
- No client-side heartbeat writes or inferred runtime identity were introduced.
- Runtime health board is read-only and workspace-scoped through existing runtime APIs.
- No new shell/process capability, allowlist expansion, or workspace-boundary weakening.

## Next Slice
`COMMANDDECK-PREVIEW-COMMAND-PROVENANCE-020` (conditional on provable linkage path)
