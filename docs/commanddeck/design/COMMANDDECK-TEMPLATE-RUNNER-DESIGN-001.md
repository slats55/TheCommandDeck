# COMMANDDECK-TEMPLATE-RUNNER-DESIGN-001

## 1. Purpose

The TemplateRunner gateway is the enforcement point that ensures CommandDeck executes commands **only** through approved, audited templates — never through raw arbitrary shell input.

It provides:
- **Pre-dispatch identity binding** — runtime and workspace must both be verified before any command executes
- **Pre-dispatch audit** — an append-only ledger entry is created before dispatch, so every command execution has a traceable audit record even on failure
- **Argument validation** — template arguments are validated against the template's allowlist before execution
- **Output redaction** — secrets, keys, and credentials are stripped from stdout/stderr before persistence
- **Timeout enforcement** — per-template timeouts are enforced server-side, not just in the daemon executor

The gateway closes the gap between the current command-runner handler and a secure, auditable, template-only execution model.

---

## 2. Current State

### 2.1 Command Execution Path

Server-side entry point: `server/internal/handler/commandrunner.go` — `HandleCommandRunnerRun` receives HTTP requests to execute commands via the `/api/v1/commandrunner/run` endpoint (Chi router, protected by `RequireCommandDeckRuntime`).

The handler:
1. Accepts `RuntimeID` and optionally `TemplateID` or a template name
2. Looks up the `db.CommandTemplate` row (by UUID or by name+workspace)
3. Creates a `db.CommandRun` row with status `"pending"`
4. Sends a `command_run:execute` WebSocket frame to the daemon via `DaemonHub`
5. Daemon (in `server/internal/daemon/cmdexec/executor.go`) validates workspace boundary with `isWithinBoundary`, checks an allowlist of safe commands, resolves the binary path, and executes with `exec.CommandContext` (30s timeout)
6. Daemon sends `command_run:result` back via WebSocket
7. Handler updates `CommandRun` with result (exit code, stdout, stderr, duration) via `UpdateCommandRunResult`
8. Handler writes a `CommandLedger` entry (best-effort — non-blocking)

### 2.2 Command Template Model

Defined in `server/pkg/db/queries/command_template.sql` and generated at `server/pkg/db/generated/command_template.sql.go`.

Template row fields: `id (UUID), workspace_id, name, command (string), description, category, allowed_args (text[]), working_dir_bound, risk_level, requires_approval (bool), is_builtin (bool), created_at, updated_at`.

Current queries: `GetCommandTemplate` (by ID), `GetCommandTemplateByName` (workspace + name + is_builtin), `ListCommandTemplates` (by workspace). No `is_enabled` or `timeout_ms` field currently exists.

### 2.3 Command Run Model

Defined in `server/pkg/db/queries/command_run.sql` and generated at `server/pkg/db/generated/command_run.sql.go`.

Run row fields: `id, workspace_id, template_id, runtime_id, issue_id, command, arguments, working_directory, status, exit_code, stdout, stderr, full_log_path, started_at, finished_at, duration_ms, initiator_type, initiator_id`.

Lifecycle: `pending` → dispatched via WebSocket → `running` → result posted back → `completed` / `failed` / `timeout`.

### 2.4 Ledger Behavior

Defined in `server/pkg/db/queries/command_ledger.sql` and generated at `server/pkg/db/generated/command_ledger.sql.go`.

`command_ledger` stores SHA-256 hashes of stdout/stderr — not raw output. Ledger entry is written **after** result arrives (best-effort, non-blocking). A failed ledger write is logged at Warn but does not block the result response.

### 2.5 Daemon Auth and Identity

`server/internal/daemon/identity.go` — daemon has a stable `DaemonID` UUID, merged from environment/config.

`server/internal/middleware/daemon_auth.go` — `DaemonAuth` middleware validates `mdt_` prefixed tokens with strict mode support (`DAEMON_AUTH_STRICT`). On strict, only `mdt_` tokens are accepted; otherwise PAT fallback is allowed.

`server/internal/daemonws/hub.go` — `ClientIdentity` carries `DaemonID, UserID, WorkspaceID, RuntimeIDs` — connection scope is set at auth time.

### 2.6 Redaction

`server/pkg/redact/redact.go` — `redact.Text()` strips: AWS keys, GitHub tokens, OpenAI/Anthropic keys, Slack tokens, JWTs, connection strings, generic credentials, home directory paths. Currently applied by the handler after result returns but before persisting to the DB.

---

## 3. Problem

The current `HandleCommandRunnerRun` has no pre-dispatch audit, no argument validation, no per-template timeout enforcement, and no disabled/enabled toggle on templates.

**Audit gap**: If a command dispatch fails at the WebSocket layer (daemon unreachable, workspace mismatch), there is no ledger entry — only an in-memory "pending" run. A failed dispatch leaves no server-side trace.

**Argument gap**: `allowed_args` exists in the schema but is never validated — the handler passes arguments through without checking them against the template's allowlist.

**Enforcement gap**: There is no `is_enabled` field on templates. A template row can be in the DB but logically disabled without any server-side enforcement.

**Timeout gap**: The daemon enforces a hard 30s timeout via `exec.CommandContext` in `executor.Execute`, but the server has no per-template timeout field and cannot enforce shorter timeouts for high-risk commands.

**Redaction gap**: Redaction is applied but not formally integrated into the command lifecycle — it is best-effort and could be accidentally bypassed if the handler changes.

---

## 4. Security Goals

| Goal | Description |
|------|-------------|
| No raw shell MVP | No free-form command strings are executed. All execution routes through a template. |
| Template-only execution | The daemon executor only receives a pre-resolved template ID and argv from the server — not arbitrary input. |
| Runtime required | A valid, online, workspace-scoped runtime must be verified before dispatch. |
| Workspace required | Template and runtime must belong to the same workspace. |
| Audit required | A ledger pre-entry must be created and committed before dispatch. Failure to pre-write blocks dispatch entirely. |
| Redaction required | stdout and stderr must be redacted before persistence or broadcast. |
| Timeout required | Each template must declare a timeout in milliseconds. Server enforces it. |
| No fake status | Command run status reflects actual execution state: pending, running, completed, failed, timeout. No fabricated outcomes. |

---

## 5. Proposed TemplateRunner Model

### 5.1 TemplateRunner Interface

The gateway is a new module at `server/internal/handler/templaterunner.go`:

```go
type TemplateRunner struct {
    Queries    *db.Queries
    DaemonHub  *daemonws.DaemonHub
    Redactor   *redact.Redactor
}

// Params encapsulates the execution request.
type Params struct {
    WorkspaceID uuid.UUID
    TemplateID  uuid.UUID  // required
    RuntimeID   uuid.UUID  // required
    IssueID     *uuid.UUID // optional
    Args        []string   // validated against template.allowed_args
}

// Result encapsulates the execution outcome.
type Result struct {
    CommandRunID uuid.UUID
    Status      string // "completed", "failed", "timeout", "dispatch_failed"
    ExitCode    *int
    Stdout      *string  // already redacted
    Stderr      *string  // already redacted
    DurationMs  *int
    StartedAt   *time.Time
    FinishedAt  *time.Time
}
```

### 5.2 Template Fields (existing + new)

**Existing fields on `command_template`:**
- `id, workspace_id, name, command, description, category, allowed_args, working_dir_bound, risk_level, requires_approval, is_builtin, created_at, updated_at`

**New fields for MVP:**
- `is_enabled bool` — default true; false means template rejects all executions
- `timeout_ms int` — per-template timeout in milliseconds; default 30000; enforced by gateway

### 5.3 Template Resolution

1. Primary: resolve by `TemplateID` UUID (explicit)
2. Fallback: resolve by template name within workspace (built-in lookup)
3. Template must exist and `is_enabled = true`
4. `risk_level` gate: caller must have appropriate permissions for the template's risk level (MVP: all authenticated callers pass, future: role-based gate)

### 5.4 Argument Validation

- If `allowed_args` is empty: no arguments allowed (MVP)
- If `allowed_args` has entries: args must match an exact entry (future: regex support)
- Validation happens server-side before dispatch

### 5.5 Runtime Binding

- Runtime must exist (`GetAgentRuntime`)
- Runtime must belong to the same workspace as the template
- Runtime status must be `online` or `busy` (from `RequireCommandDeckRuntime`)

### 5.6 Workspace Binding

- Template's `workspace_id` must match the runtime's `workspace_id`
- `working_dir_bound` must be set and prefix-checked against the runtime's `worktree_root` (via `isWithinBoundary`)

### 5.7 Redaction Policy

- Apply `redact.Text()` to stdout and stderr **after** result arrives but **before** writing to `CommandRun`
- Redaction must never block command execution — on failure, log and proceed with raw output

### 5.8 Audit Behavior

- Ledger pre-entry (status = `"pending"`) must be committed before WebSocket dispatch
- Ledger finalization happens after result arrives (best-effort, non-blocking)
- Pre-entry failure blocks dispatch (hard block)

---

## 6. Proposed Request Lifecycle

The `TemplateRunner.Execute` method enforces this 14-step lifecycle:

**Step 1 — Authenticate**
Caller presents valid auth token via `RequireAuth` middleware. Token identifies user and workspace.

**Step 2 — Authorize**
Check caller has permission to execute commands in the workspace (MVP: all authenticated members pass; future: role-based for high-risk templates).

**Step 3 — Resolve Template**
By `TemplateID` UUID. If missing, return `404`. Lookup via `GetCommandTemplate`.

**Step 4 — Validate Template Enabled**
Check `template.is_enabled == true`. If false, return `403` — no run created, no ledger entry.

**Step 5 — Validate Arguments**
Parse `Params.Args` against `template.allowed_args`:
- If `allowed_args` is empty: reject any non-empty args
- If `allowed_args` has entries: args must match exactly (no wildcards in MVP)

If invalid, return `400`, no run created.

**Step 6 — Validate Runtime**
Call `RequireCommandDeckRuntime` to verify:
- Runtime exists
- Runtime belongs to the same workspace as the template
- Runtime status is `online` or `busy`

If invalid, return `400`, no run created.

**Step 7 — Validate Workspace Boundary**
Call `isWithinBoundary(template.working_dir_bound, runtime.worktree_root)`. If false, return `400`, no run created.

**Step 8 — Create Ledger Pre-Entry**
Call `CreateCommandLedgerEntry` with `status = "pending"`, `created_at = now()`. If this fails, return `500`, hard block — no command_run created, no WS dispatch.

**Step 9 — Create CommandRun**
Call `CreateCommandRun` with `status = "pending"`. Insert failure: return `500`, ledger entry remains (will be cleaned up by future reconciliation job).

**Step 10 — Dispatch to Daemon**
Send `command_run:execute` frame via `DaemonHub.SendToRuntime(RuntimeID, frame)`. Frame carries: `RunID`, `Command` (from template), `Args`, `WorkingDirectory`, `TimeoutMs`. If send fails, mark run `dispatch_failed`, return `502`.

**Step 11 — Await Result**
Wait for `command_run:result` WebSocket frame with per-template `timeout_ms`. If timeout fires, mark run `timeout`. If daemon disconnects, mark run `daemon_disconnected`.

**Step 12 — Redact Output**
Apply `redact.Text()` to raw stdout and stderr. On redaction error: log at Warn, proceed with raw output.

**Step 13 — Update CommandRun**
Call `UpdateCommandRunResult` with: `status`, `exit_code`, `stdout` (redacted), `stderr` (redacted), `duration_ms`, `started_at`, `finished_at`.

**Step 14 — Finalize Ledger**
Attempt to update ledger entry with: final `status`, `exit_code`, `duration_ms`, `stdout_hash`, `stderr_hash`. On failure: log at Warn, do not block. Ledger finalization is best-effort — the authoritative result record is `CommandRun`.

---

## 7. Failure States

| Failure | Detection Point | Response | Severity |
|---|---|---|---|
| Invalid template ID | Step 3 | Return `404`, no run created, no ledger entry | Error |
| Template disabled (`is_enabled=false`) | Step 4 | Return `403`, no run created, no ledger entry | Error |
| Invalid arguments | Step 5 | Return `400`, no run created, no ledger entry | Error |
| Runtime offline | Step 6 | Return `400`, no run created, no ledger entry | Error |
| Workspace missing | Step 7 | Return `400`, no run created, no ledger entry | Error |
| Workspace boundary failure | Step 7 | Return `400`, no run created, no ledger entry | Error |
| Ledger pre-write failure | Step 8 | Return `500`, no run created, hard block | Critical |
| Command run create failure | Step 9 | Return `500`, ledger entry orphaned (reconciliation job cleans) | Critical |
| WS dispatch failure | Step 10 | Mark run `dispatch_failed`, return `502` | High |
| Command timeout | Step 11 | Mark run `timeout`, finalize ledger | Medium |
| Command cancel | Step 11 | Mark run `cancelled`, finalize ledger | Medium |
| Daemon disconnect | Step 11 | Mark run `daemon_disconnected`, finalize ledger | Medium |
| Redaction failure | Step 12 | Log Warn, proceed with raw output (never block) | Low |
| Result write failure | Step 13 | Log error, run stays in current state | High |
| Ledger finalize failure | Step 14 | Log Warn, do not block (best-effort) | Low |

---

## 8. API Boundary Proposal

This section defines the API shape — no implementation required in this design.

### 8.1 Execute Command (existing endpoint, enhanced behavior)

**Endpoint:** `POST /api/v1/commandrunner/run`

**Request body:**
```json
{
  "runtime_id": "uuid",
  "template_id": "uuid",
  "issue_id": "uuid (optional)",
  "args": ["arg1", "arg2"] // optional, validated against template.allowed_args
}
```

**Response (200 OK):**
```json
{
  "id": "uuid",
  "status": "pending",
  "command": "git status",
  "working_directory": "/home/user/work",
  "created_at": "RFC3339"
}
```

**Error responses:** `400` (invalid args/runtime/workspace), `403` (template disabled), `404` (template not found), `500` (internal error), `502` (daemon dispatch failed).

### 8.2 List Command Runs

**Endpoint:** `GET /api/v1/commandrunner/runs?runtime_id=uuid&limit=50&offset=0`

Unchanged from current behavior — returns list of runs for the runtime.

### 8.3 Get Command Run

**Endpoint:** `GET /api/v1/commandrunner/run/{run_id}`

Unchanged from current behavior.

### 8.4 List Templates

**Endpoint:** `GET /api/v1/commandrunner/templates?workspace_id=uuid`

Returns templates available in the workspace. Response includes `is_enabled` and `timeout_ms` fields.

### 8.5 WebSocket Stream

**Path:** `/ws/daemon` (existing, unchanged)

The WebSocket protocol is unchanged — `command_run:execute` frame dispatches, `command_run:result` returns. The server-side gateway (Step 10–11) is the new enforcement layer.

---

## 9. Service Boundary Proposal

**Recommended location:** `server/internal/handler/templaterunner.go`

**Rationale:**
- The handler layer already orchestrates auth, template resolution, runtime validation, DB writes, and WS dispatch
- The daemon layer is a passive executor (receive frame → execute → return) — it must not accumulate business logic
- A new service layer (`server/internal/service/`) would add indirection without benefit at MVP scale
- The handler can enforce all preconditions (Steps 1–8) before WebSocket dispatch without architectural churn

The existing `HandleCommandRunnerRun` in `commandrunner.go` becomes a thin wrapper:

```go
func (h *Handler) HandleCommandRunnerRun(w http.ResponseWriter, r *http.Request) {
    runner := &TemplateRunner{
        Queries:   h.Queries,
        DaemonHub: h.DaemonHub,
        Redactor:  h.Redactor,
    }
    params := buildParamsFromRequest(r)
    result := runner.Execute(r.Context(), params)
    encodeResult(w, result)
}
```

**Dependency graph:**
```
HTTP Request
  → RequireAuth middleware (existing)
  → HandleCommandRunnerRun (existing wrapper)
  → TemplateRunner.Execute (new)
    → h.Queries (db.Queries, existing)
    → h.DaemonHub (daemonws.DaemonHub, existing)
    → h.Redactor (redact.Redactor, existing)
    → daemon WS dispatch (existing)
```

---

## 10. Database / Ledger Impact

### 10.1 Current Schema

The current `command_template`, `command_run`, and `command_ledger` tables have sufficient fields for MVP. No schema changes are required for the core lifecycle.

### 10.2 New Template Fields

Two new fields on `command_template` for MVP:

```sql
ALTER TABLE command_template ADD COLUMN is_enabled bool NOT NULL DEFAULT true;
ALTER TABLE command_template ADD COLUMN timeout_ms int NOT NULL DEFAULT 30000;
```

This is a forward-only migration — no existing data is invalidated. `is_enabled` defaults to `true` so existing templates remain functional.

### 10.3 Ledger Pre-Entry Pattern

Current behavior: ledger entry created **after** result arrives (best-effort).

New behavior: ledger entry created **before** dispatch (hard block) + updated **after** result arrives (best-effort).

This requires a `status` field on `command_ledger` — already present (`pending`, `completed`, `failed`). The pre-entry uses `status = "pending"` and finalization updates to `completed`/`failed`/`timeout`.

### 10.4 Future Schema Additions (out of scope for MVP)

- Template version numbering (for template update safety)
- Per-run argument hash (for audit reproducibility)
- Dispatch failure reason field on `command_run` (currently only status enum)

---

## 11. Out of Scope

The following are explicitly excluded from this design and the first implementation slice:

| Exclusion | Reason |
|---|---|
| Raw browser terminal | Security: bypasses template gateway entirely |
| Arbitrary command input | Security: defeats purpose of approved template model |
| Preview registry | Future work — template CRUD and discovery UI |
| UI polish | Frontend work, not server-side gateway work |
| Broad daemon rewrite | Daemon remains a passive executor; business logic stays server-side |
| Docker/devcontainer expansion | Out of scope for MVP command runner |
| GitHub Actions bridge | Future integration work |
| Frontend UI at `/commanddeck` | This design covers the server-side gateway only |
| Multi-tenant template catalogs | Future namespacing work |
| User-controlled executable paths | Security: hard-banned in MVP |
| User-controlled shell strings | Security: hard-banned in MVP |
| Argument regex validation | Future: MVP uses exact-match or no-args |

---

## 12. First Build Slice Recommendation

**Task ID:** COMMANDDECK-TEMPLATE-RUNNER-001

**Objective:** Create `server/internal/handler/templaterunner.go` with the `TemplateRunner` struct and `Execute` method. Implement the 14-step lifecycle from template resolution through ledger pre-write and result finalization. Do not build the frontend.

**Scope:**
1. Create `server/internal/handler/templaterunner.go`
2. Define `TemplateRunner`, `Params`, and `Result` types
3. Implement `Execute(ctx, params) → Result` covering all 14 steps
4. Wire the gateway into `HandleCommandRunnerRun` (thin wrapper pattern)
5. Add `is_enabled` and `timeout_ms` migration for `command_template`
6. Add `is_enabled` and `timeout_ms` fields to the generated model (sqlc regeneration)
7. Implement ledger pre-write (hard block) before WS dispatch
8. Apply `redact.Text()` to stdout/stderr before `UpdateCommandRunResult`
9. Add `dispatch_failed` status to `command_run` status enum
10. No frontend changes

**Not in scope for this slice:**
- Frontend React components
- Template CRUD endpoints
- Preview registry
- Argument regex validation
- Role-based permission on risk_level

**Verification:**
- `HandleCommandRunnerRun` continues to work for existing callers
- Ledger pre-entry is created before any WS dispatch
- Ledger pre-entry failure blocks dispatch with 500
- `is_enabled=false` template rejects with 403
- Timeout is enforced server-side
- Stdout/stderr are redacted before DB write