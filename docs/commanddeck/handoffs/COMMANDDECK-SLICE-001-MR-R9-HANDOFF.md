# COMMANDDECK-SLICE-001: git status Command Runner — Mr.R9 Handoff

**Agent:** Mr.R9
**Date:** 2025-05-21
**Branch:** `feature/commanddeck-slice-001-git-status-runner` (from `origin/chore/commanddeck-discovery-001`)
**Status:** ✅ Implemented, not yet pushed or tested

---

## What Was Built

A safe `git status` command runner with full HTTP API surface and daemon-side WebSocket wiring.

### Files Created

| File | Purpose |
|------|---------|
| `server/migrations/084_command_template.up.sql` | `command_template` table — allowlist of approved commands |
| `server/migrations/084_command_template.down.sql` | Rollback for 084 |
| `server/migrations/085_command_run.up.sql` | `command_run` table — execution tracking |
| `server/migrations/085_command_run.down.sql` | Rollback for 085 |
| `server/pkg/protocol/command_run.go` | `CommandRunExecutePayload`, `CommandRunResultPayload`, protocol constants |
| `server/pkg/db/queries/command_template.sql` | 3 queries: `GetCommandTemplate`, `GetCommandTemplateByName`, `ListCommandTemplates` |
| `server/pkg/db/queries/command_run.sql` | 4 queries: `CreateCommandRun`, `GetCommandRun`, `ListCommandRuns`, `UpdateCommandRunResult` |
| `server/internal/handler/commandrunner.go` | HTTP handler (4 endpoints) + `HandleDaemonCommandRunWS` for daemon→server WS result |
| `server/internal/daemon/cmdexec/executor.go` | Safe executor: only `git status` allowed, argv-style exec, workspace boundary check |
| `server/internal/daemon/cmdexec/daemon.go` | `WebSocketHandler` bridging WS messages to executor |

### Files Modified

| File | Change |
|------|--------|
| `server/internal/daemonws/hub.go` | Added `CommandRunHandler` type, `onCommandRun` field, `SetCommandRunHandler()`, `handleCommandRunFrame()` |
| `server/cmd/server/router.go` | Registered 4 routes under `/api/commandrunner`, wired `daemonHub.SetCommandRunHandler(h.HandleDaemonCommandRunWS)` |
| `server/internal/daemon/daemon.go` | Added `cmdexecHandler` field, `SetCommandRunHandler()` method |
| `server/internal/daemon/wakeup.go` | Added `protocol.CommandRunExecute` case in `readTaskWakeupMessages` switch, wired `d.SetCommandRunHandler(writes)` after WS connection |

---

## Architecture

```
Client (browser / agent)
    │
    │ HTTP POST /api/commandrunner/run
    ▼
┌─────────────────────────────────────────────────────────┐
│  Handler (commandrunner.go)                             │
│   - Validates runtime is online                         │
│   - Looks up command template (or defaults to Git Status)│
│   - Creates command_run record (status=pending)         │
│   - Sends command_run:execute frame via DaemonHub      │
└─────────────────────────────────────────────────────────┘
    │
    │ daemonHub.DeliverDaemonRuntime(runtimeID, frame)
    ▼
┌─────────────────────────────────────────────────────────┐
│  Hub (hub.go) → notifies client watching runtimeID     │
└─────────────────────────────────────────────────────────┘
    │
    │ WebSocket (same task-wakeup connection)
    ▼
┌─────────────────────────────────────────────────────────┐
│  Daemon (wakeup.go)                                    │
│   - Receives command_run:execute in readPump           │
│   - Routes to cmdexecHandler.Handle()                  │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  cmdexec WebSocketHandler (cmdexec/daemon.go)          │
│   - Unmarshals payload                                  │
│   - Calls executor.Execute()                           │
│   - Sends command_run:result on WS write queue         │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  cmdexec Executor (cmdexec/executor.go)                │
│   - Validates working_dir within workspace boundary     │
│   - Parses command to ["git","status"]                 │
│   - Checks allowlist                                   │
│   - exec.CommandContext with 30s timeout              │
│   - Returns Result{Status, ExitCode, Stdout, Stderr}  │
└─────────────────────────────────────────────────────────┘
    │
    │ WebSocket
    ▼
┌─────────────────────────────────────────────────────────┐
│  Hub.handleCommandRunFrame (hub.go)                   │
│   - Calls HandleDaemonCommandRunWS                     │
│   - Updates DB with result                             │
└─────────────────────────────────────────────────────────┘
```

---

## HTTP API

### `GET /api/commandrunner/templates`
Returns command templates available for the workspace.

**Response:**
```json
{
  "templates": [
    {
      "id": "uuid",
      "name": "Git Status",
      "command": "git status",
      "description": "Show the working tree status",
      "category": "git",
      "risk_level": "low",
      "is_builtin": true,
      "created_at": "2025-05-21T00:00:00Z"
    }
  ]
}
```

### `POST /api/commandrunner/run`
Execute a command on a runtime.

**Request:**
```json
{
  "runtime_id": "uuid",
  "template_id": "uuid",   // optional, defaults to "Git Status"
  "issue_id": "uuid"       // optional
}
```

**Response (201):**
```json
{
  "id": "uuid",
  "status": "pending",
  "command": "git status",
  "working_directory": "/home/user/multica_workspaces/...",
  "created_at": "..."
}
```

### `GET /api/commandrunner/run/{runId}`
Get a single command run by ID.

### `GET /api/commandrunner/runs`
List all command runs for the workspace.

---

## WebSocket Protocol

### Server → Daemon: `command_run:execute`
```json
{
  "type": "command_run:execute",
  "payload": {
    "command_run_id": "uuid",
    "runtime_id": "uuid",
    "command": "git status",
    "working_directory": "/home/user/workspaces/abc",
    "allowed_dir": "/home/user/workspaces",
    "workspace_id": "uuid",
    "initiator_type": "member",
    "initiator_id": "uuid",
    "issue_id": "uuid"
  }
}
```

### Daemon → Server: `command_run:result`
```json
{
  "type": "command_run:result",
  "payload": {
    "command_run_id": "uuid",
    "status": "completed",
    "exit_code": 0,
    "stdout": "On branch main\nnothing to commit, working tree clean",
    "stderr": "",
    "duration_ms": 142
  }
}
```

---

## Security Properties

1. **Allowlist only** — only `git status` is permitted; all other commands rejected
2. **No shell** — uses `exec.CommandContext` with argv split at parse time (no string splitting)
3. **Workspace boundary** — working directory validated to stay under `WorkspacesRoot`
4. **Timeout** — 30-second hard limit on execution
5. **No secrets** — inherits daemon environment only, no extra credentials injected

---

## Pending / Blocked

1. **`go build ./...`** — has not been run; compilation errors unknown
2. **`sqlc generate`** — SQL query files written but not compiled into Go
3. **`cmdexecHandler` nil-check gap** — if `SetCommandRunHandler` is never called (WS never connects), handler is nil; the switch case handles this gracefully
4. **No unit tests** for `cmdexec/executor.go` or handler methods

---

## Notes for Reviewer

- `commandrunner.go` uses `util.TextToPtr` — confirmed available in `internal/util/pgx.go`
- `commandrunner.go` uses `util.MustParseUUID`, `util.UUIDToString` — confirmed available
- `handleCommandRunnerRun` passes `worktreeRoot` (from runtime metadata key `worktree_root`) as both `WorkingDirectory` and `AllowedDir`; the executor uses `AllowedDir` as the boundary for validation
- The WS write channel (`writes`) in `wakeup.go` is buffered (size computed from `2*len(runtimeIDs)`, min 16) — result frames are dropped if buffer is full (logged at debug level)
- Built-in "Git Status" template has `name="Git Status"`, `command="git status"`, `is_builtin=true` — seeded by migration or separately
- `protocol.Message` type was not added to `protocol/command_run.go` (it was in an earlier draft) — `Message` lives in `protocol/messages.go` and is already used by the daemon WS framing