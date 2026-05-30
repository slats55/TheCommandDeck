# COMMANDDECK-COMMAND-RUN-LIVE-EVENTS-018

## Task ID
`COMMANDDECK-COMMAND-RUN-LIVE-EVENTS-018`

## Branch
`feature/commanddeck-command-run-live-events-018`

## Base
`origin/main` at `a7e4a3b773fc073fbf012031ba8b384765a62312` (structured evidence `017` merged)

## Objective
Deliver truthful, workspace-scoped live command-run lifecycle events into CommandDeck UI without removing safe polling fallback.

## Implementation Summary

### Backend transport and lifecycle wiring
- Added daemon lifecycle frame:
  - `command_run:started` (`protocol.CommandRunStarted`)
  - payload: `command_run_id`, `status`
- Daemon command executor now emits:
  1. `command_run:started` when execution begins
  2. `command_run:result` when execution completes
- Server daemon WS hub now:
  - accepts `command_run:started` with dedicated handler wiring
  - relays `command_run:cancel` frames by runtime ID (runtime extraction support added)
- Router wiring now registers both:
  - `SetCommandRunStartedHandler(h.HandleDaemonCommandRunStartedWS)`
  - `SetCommandRunHandler(h.HandleDaemonCommandRunWS)`

### Command-run state/event publication
- Command-run handlers now publish workspace events (`command_run:updated`) on:
  - run creation (`pending`)
  - daemon start (`running`)
  - cancellation request metadata update
  - final daemon result (`completed|failed|timeout|cancelled`)
- Start handler updates DB state from `pending` -> `running` with server-owned `started_at`.
- Final result handler preserves structured truncation fields and publishes updated run payload.

### Frontend live update path
- Added `command_run:updated` to core WS event union.
- Added `CommandRunUpdatedPayload` type contract in `packages/core/types/events.ts`.
- `useRealtimeSync` now invalidates CommandDeck run-history cache on `command_run:*` events and on reconnect:
  - query key: `["commanddeck","runs",wsId]`
- CommandDeck page now subscribes to `command_run:updated` and applies validated payloads to run history cache for immediate UI updates.
- Strict payload validation added in page:
  - malformed payloads are ignored
  - no unsafe UI state mutation from unvalidated event data

## Files Changed
- `server/pkg/protocol/command_run.go`
- `server/internal/daemon/cmdexec/daemon.go`
- `server/internal/daemon/cmdexec/daemon_test.go`
- `server/internal/daemonws/hub.go`
- `server/internal/daemonws/hub_test.go`
- `server/internal/handler/commandrunner.go`
- `server/cmd/server/router.go`
- `packages/core/types/events.ts`
- `packages/core/realtime/use-realtime-sync.ts`
- `apps/web/app/[workspaceSlug]/(dashboard)/commanddeck/page.tsx`
- `apps/web/app/[workspaceSlug]/(dashboard)/commanddeck/page.test.tsx`
- `docs/commanddeck/02-ROADMAP.md`

## Verification Executed
- Backend:
  - `docker run --rm -v "${PWD}/server:/src" -w /src golang:1.26 /bin/sh -lc "export PATH=/usr/local/go/bin:$PATH; go test ./internal/daemon/cmdexec ./internal/daemonws ./internal/handler ./cmd/server"`
  - `docker run --rm -v "${PWD}/server:/src" -w /src golang:1.26 /bin/sh -lc "export PATH=/usr/local/go/bin:$PATH; go build ./cmd/server"`
- Frontend:
  - `pnpm --filter @multica/web test -- "app/[workspaceSlug]/(dashboard)/commanddeck/page.test.tsx"`
  - `pnpm lint`
  - `pnpm test`
  - `pnpm build`
- Local health probes:
  - `http://localhost:8080/health` -> 200
  - `http://localhost:3000` -> 200
  - `http://localhost:3000/login` -> 200

## Security Notes
- Events are published through existing workspace-scoped event bus/hub path.
- No new raw shell/process control surface introduced.
- Existing command allowlist and workspace runtime checks remain unchanged.
- UI ignores malformed live event payloads.

## Next Slice
`feature/commanddeck-runtime-health-heartbeat-offline-019`
