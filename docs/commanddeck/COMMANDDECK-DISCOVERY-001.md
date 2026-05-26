# CommandDeck Discovery Report — COMMANDDECK-DISCOVERY-001

**Date:** 2026-05-20
**Author:** Mr.Commander
**Branch:** `chore/commanddeck-discovery-001`
**Base Branch:** `main`
**Status:** Complete — Awaiting Approval

---

## 1. Current Repo Architecture Map

The Multica fork (`slats55/multica`) is a monorepo with three main layers:

### Layer 1: Frontend (TypeScript — ~104K LOC)

| Directory | Purpose | Key Files |
|-----------|---------|-----------|
| `apps/web/` | Next.js 16 App Router frontend | `app/`, `platform/` (Next.js API routes) |
| `packages/core/` | Headless business logic | Zustand stores, React Query hooks, API client |
| `packages/ui/` | Atomic UI components (shadcn/Base UI) | Zero business logic |
| `packages/views/` | Shared business pages | Runtime list, agent settings, issues board |

**Critical sub-packages for CommandDeck:**
- `packages/core/runtimes/` — runtime health derivation, hooks, models, mutations
- `packages/core/realtime/` — WebSocket provider + sync hooks for browser clients
- `packages/core/api/` — API client
- `packages/views/runtimes/` — Runtime list UI components

### Layer 2: Backend (Go — ~122K LOC)

| Directory | Purpose | Key Files |
|-----------|---------|-----------|
| `server/cmd/server/` | Main server binary | Starts HTTP + WS, runs migrations |
| `server/internal/handler/` | HTTP handlers (Chi router) | `daemon.go`, `daemon_ws.go`, `runtime.go`, `handler.go` |
| `server/internal/daemon/` | Local daemon (runs on agent machines) | `daemon.go`, `types.go`, `client.go` |
| `server/internal/daemonws/` | Daemon WebSocket hub | `hub.go` — per-runtime WS connections |
| `server/internal/realtime/` | Browser WebSocket hub | `hub.go`, `broadcaster.go` — per-workspace pub/sub |
| `server/internal/service/` | Service layer | `task.go`, `autopilot.go` |
| `server/migrations/` | PostgreSQL migrations (83+) | Sequential up/down SQL files |

### Layer 3: Infrastructure

| File | Purpose |
|------|---------|
| `docker-compose.selfhost.yml` | Primary self-host stack (postgres + backend + frontend) |
| `docker-compose.selfhost.build.yml` | Source-build variant (local images instead of GHCR pulls) |
| `Dockerfile` | Backend server image |
| `Dockerfile.web` | Frontend web image |
| `Makefile` | Dev, test, build commands |

---

## 2. Frontend/Backend/Daemon File Areas Likely Involved

### Frontend

| Area | Files | Purpose for CommandDeck |
|------|-------|------------------------|
| Runtime selector UI | `packages/views/runtimes/*` | Already lists runtimes — extend to select a runtime for command execution |
| Command runner view | **NEW** `packages/views/commanddeck/` | Command form, template selector, output stream, history |
| Preview registry view | **NEW** `packages/views/commanddeck/previews/` | Preview cards with status, URLs, controls |
| Core commands store | **NEW** `packages/core/commanddeck/` | Zustand store + React Query hooks for commands/previews |
| Navigation | `packages/views/navigation/` | Add CommandDeck nav item |
| Realtime sync | `packages/core/realtime/` | Subscribe to command-output WS events |

### Backend

| Area | Files | Purpose for CommandDeck |
|------|-------|------------------------|
| Command template CRUD | **NEW** `server/internal/handler/commandrunner.go` | API endpoints for templates, execution, history |
| Command execution | **NEW** endpoint in daemon handler or new handler | Accept command requests, relay to daemon, stream output |
| Daemon command executor | **NEW** in `server/internal/daemon/` | Execute approved commands locally, stream stdout/stderr back |
| Preview registry | **NEW** `server/internal/handler/preview_registry.go` | CRUD for preview URLs |
| Database models | **NEW** migrations + `server/pkg/db/queries/` | command_templates, command_runs, previews tables |
| WebSocket streaming | Extend `daemonws/` or use existing `realtime/` | Stream command output to browser clients |

### Daemon

| Area | Files | Purpose for CommandDeck |
|------|-------|------------------------|
| Command execution | **NEW** in `server/internal/daemon/cmdexec/` | Execute sanctioned commands, capture output, enforce boundaries |
| Existing health endpoint | `server/internal/daemon/health.go` | Already responds — could add command acceptance endpoint |
| Working directory isolation | `server/internal/daemon/execenv/` | Reuse repo/worktree isolation patterns |

---

## 3. Existing Task/Runtime/Log Streaming Flow

### Current Flow (AI Agent Task Execution)

```
1. User assigns issue to agent (via web UI)
2. Server creates entry in agent_task_queue (status: queued)
3. Daemon polls ClaimTask endpoint for its runtime
4. Server claims task (status: dispatched → running)
5. Daemon prepares isolated execution environment (execenv)
6. Daemon spawns AI agent CLI (Claude Code / Codex / etc.) with prompt
7. AI agent works in isolated workdir, calls `multica` CLI for repo/task ops
8. Daemon watches task cancellation (5s poll)
9. Agent finishes → daemon reports CompleteTask or FailTask
10. Result (comment, branch, session_id, work_dir) stored in DB
```

### Existing WebSocket Architecture

```
Browser ←ws→ realtime.Hub (per-workspace pub/sub)
Daemon  ←ws→ daemonws.Hub (per-runtime connections)

realtime.Hub: delivers issue/comments/member events to browser clients
daemonws.Hub: delivers task_available wakeups to daemons; receives heartbeats
```

### Where Command Output Would Fit

The daemon currently executes AI agents, not arbitrary user-initiated commands. For CommandDeck, we need a **new path**:

```
Browser → Backend API → Daemon (command execution) → stdout/stderr stream → WebSocket → Browser
```

The `daemonws.Hub` already maintains per-runtime WS connections, which is the natural path for streaming command output back.

---

## 4. Command Execution: Backend, Daemon, or Both?

**Recommendation: Backend routes + Daemon executes + WebSocket streams.**

| Layer | Responsibility |
|-------|---------------|
| **Backend** | Auth, allowlist policy, template management, routing request to correct daemon, persisting history |
| **Daemon** | Actual command execution, working directory enforcement, output streaming, timeout/kill |
| **WebSocket** | Stream stdout/stderr from daemon → backend → browser in real-time |

**Why not backend-only:**
- The backend runs in Docker. It doesn't have access to agent machines' filesystems, repos, or toolsets.
- Command execution must happen on the runtime machine where repos are cloned.

**Why not daemon-only:**
- The daemon has no UI. The user needs a browser interface.
- Auth, policy, and persistence need the backend.

---

## 5. Recommended Data Model for Command Runs

### `command_template` table

```sql
CREATE TABLE command_template (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    name TEXT NOT NULL,                    -- e.g. "Git Status"
    command TEXT NOT NULL,                 -- e.g. "git status"
    description TEXT,                      -- Human explanation
    category TEXT NOT NULL DEFAULT 'general',  -- git, docker, npm, python, etc.
    allowed_args TEXT[] DEFAULT '{}',      -- Optional validated arguments
    working_dir_bound TEXT,                -- Must be within this prefix
    risk_level TEXT NOT NULL DEFAULT 'low' CHECK (risk_level IN ('low', 'medium', 'high', 'blocked')),
    requires_approval BOOLEAN NOT NULL DEFAULT false,
    is_builtin BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### `command_run` table

```sql
CREATE TABLE command_run (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    template_id UUID REFERENCES command_template(id) ON DELETE SET NULL,
    runtime_id UUID NOT NULL REFERENCES agent_runtime(id) ON DELETE CASCADE,
    issue_id UUID REFERENCES issue(id) ON DELETE SET NULL,
    
    command TEXT NOT NULL,                 -- The exact command executed
    arguments TEXT[] DEFAULT '{}',        -- Sanitized arguments
    working_directory TEXT NOT NULL,      -- Bounded path on the runtime machine
    
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'completed', 'failed', 'timeout', 'cancelled')),
    
    exit_code INT,
    stdout TEXT,                          -- Truncated at 10K chars
    stderr TEXT,                          -- Truncated at 10K chars
    full_log_path TEXT,                   -- Path to complete log on runtime machine
    
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    duration_ms INT,
    
    initiator_type TEXT NOT NULL CHECK (initiator_type IN ('member', 'agent')),
    initiator_id UUID NOT NULL,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### `preview` table

```sql
CREATE TABLE preview (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    runtime_id UUID NOT NULL REFERENCES agent_runtime(id) ON DELETE CASCADE,
    command_run_id UUID REFERENCES command_run(id) ON DELETE SET NULL,
    
    app_name TEXT NOT NULL,
    port INT NOT NULL,
    url TEXT,                              -- Constructed or provided
    start_command TEXT,
    
    status TEXT NOT NULL DEFAULT 'unknown'
        CHECK (status IN ('unknown', 'running', 'stopped', 'failed')),
    
    last_checked_at TIMESTAMPTZ,
    last_error TEXT,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

---

## 6. Security Risks and Mitigations

### Risk 1: Arbitrary Command Injection
**Severity:** Critical
**Mitigation:** All commands must go through a command template allowlist. No free-form command entry in the MVP. Templates are validated server-side. Arguments are sanitized and validated against `allowed_args`.

### Risk 2: Working Directory Escape
**Severity:** Critical
**Mitigation:** Every command run has a `working_directory` bounded to the workspace root on the runtime machine. The daemon must enforce `working_dir_bound` before execution. `cd ..` and symlink traversal must be caught.

### Risk 3: Unauthenticated Command Execution
**Severity:** High
**Mitigation:** All command endpoints require authentication. Workspace membership is verified before routing to a daemon. Runtime selection is scoped to the user's workspace.

### Risk 4: Secret Leakage in Logs
**Severity:** High
**Mitigation:** Implement a redact filter (similar to `server/pkg/redact/`) on command output before persistence. Strip `Authorization`, `Bearer`, `API_KEY`, `token`, `password` patterns from stdout/stderr.

### Risk 5: Denial of Service via Long-Running Commands
**Severity:** Medium
**Mitigation:** Default timeout per command template. Max timeout configurable per risk level. User-visible cancel button forces daemon kill.

### Risk 6: Lateral Movement via Docker Commands
**Severity:** Medium
**Mitigation:** Docker commands are `risk_level = medium` and require approval. `docker exec`, `docker run --privileged` etc. are blocked/`risk_level = blocked`.

---

## 7. First 3 Implementation Slices After Discovery

### Slice 1: Command Runner Foundation (COMMANDDECK-SLICE-001)

**Goal:** One approved command (`git status`) executes on one runtime and returns output.

**Deliverables:**
1. `command_template` table + migration
2. `command_run` table + migration
3. Backend API endpoints: `POST /api/commandrunner/run`, `GET /api/commandrunner/run/{id}`, `GET /api/commandrunner/runs`
4. Daemon command executor (new module in `server/internal/daemon/cmdexec/`)
5. Backend → daemon command relay (new daemon HTTP endpoint or WS message type)
6. Stdout/stderr streamed via WebSocket to browser
7. Minimal frontend: runtime selector + command button + output pane
8. Built-in templates seeded for safe git commands

**Out of scope:** Preview registry, custom templates, command arguments, multiple runtimes

**Branch:** `feat/commanddeck-slice-001`

**Acceptance Criteria:**
- User selects a runtime from CommandDeck UI
- User clicks "git status" from approved command list
- Command executes on the remote daemon
- Output streams to the browser in real-time
- Exit code is displayed
- Command is saved to history
- Working directory is bounded to workspace repo root
- No free-form command entry exists

### Slice 2: Command Templates + History (COMMANDDECK-SLICE-002)

**Goal:** Multiple approved templates, command history, and working directory context.

**Deliverables:**
1. Template CRUD UI (admin-only)
2. Category grouping (git, npm, docker)
3. Command argument support (validated against `allowed_args`)
4. Full command history page with filtering
5. Per-command detail view with full log access
6. Working directory auto-detection from repo context
7. `git diff --stat`, `npm run build`, `npm test` templates

**Branch:** `feat/commanddeck-slice-002`

### Slice 3: Preview Registry MVP (COMMANDDECK-SLICE-003)

**Goal:** Track dev server previews started by command runs.

**Deliverables:**
1. `preview` table + migration
2. Preview CRUD API
3. "Start Dev Server" command template → auto-creates preview entry
4. Preview card UI with status, port, URL
5. Stop/restart actions
6. Health check (poll port on runtime machine)

**Branch:** `feat/commanddeck-slice-003`

---

## 8. Acceptance Criteria for Slice 1

1. **Branch:** `feat/commanddeck-slice-001` off `main`
2. **Migration:** Two new SQL migration files for `command_template` and `command_run`
3. **Backend:** New handler file `server/internal/handler/commandrunner.go` with:
   - `POST /api/commandrunner/run` — accepts `{runtime_id, template_id, issue_id?}`, validates against allowlist, dispatches to daemon
   - `GET /api/commandrunner/run/{id}` — returns command status + output
   - `GET /api/commandrunner/runs` — lists command history for workspace
4. **Daemon:** New `server/internal/daemon/cmdexec/executor.go`:
   - Accepts command execution request from backend
   - Enforces working directory boundary
   - Executes with timeout
   - Streams stdout/stderr back via WebSocket or polling
5. **Frontend:** New route `/commanddeck`:
   - Runtime selector dropdown (reuse existing runtime list)
   - Approved command button list (starts with `git status`)
   - Output pane (scrollable, auto-scrolls to bottom)
   - Exit code display
   - Basic history (last 10 runs)
6. **Security:**
   - All endpoints require workspace auth
   - Template allowlist enforced server-side
   - Working directory bounded to workspace root
   - No free-form command input
7. **Evidence:**
   - `git status` output matches what `git status` returns on the runtime machine
   - Exit code is 0 for success, non-zero for failure
   - Output is identical across two consecutive runs
   - Command appears in history after execution

---

## 9. Recommended Agent Assignments

### For COMMANDDECK-SLICE-001:

| Agent | Role | Assignment |
|-------|------|-----------|
| **Mr.R9** | Primary Builder | Build the backend + daemon components: migrations, handler, cmdexec module, WS streaming |
| **Mr.R7** | Independent Verifier | After Mr.R9 delivers, verify: fetch branch, run migrations, test `git status` on actual runtime, confirm output matches, confirm no security bypass |
| **Mr.M1** | Gatekeeper | Final review: confirm all AC met, diff scope clean, no fake data, merge approval |

---

## 10. Exact Next Action

**Wait for Myles to approve this discovery document.**

After approval, Mr.Commander will assign COMMANDDECK-SLICE-001 to Mr.R9 with exact build prompts.

**Do not start coding until approved.**

---

## 11. Repository LOC Summary

| Language | Lines | Component |
|----------|-------|-----------|
| Go | ~122K | Backend + Daemon + CLI |
| TypeScript/TSX | ~104K | Frontend (apps + packages) |
| SQL | Extensive | 83+ migrations |
| YAML/Docker | ~50 files | Config, CI, Compose |

---

*End of COMMANDDECK-DISCOVERY-001*
