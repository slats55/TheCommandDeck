# CommandDeck Commercial Delivery Roadmap

## Product Mission

CommandDeck is a secure self-hosted operational control plane for agent-assisted software development, command execution, preview lifecycle, workflow governance, and deployment evidence.

## Release Tracks

### R0.1 - Secure Run Control Plane

Required capabilities:
- approved command templates
- workspace boundary enforcement
- runtime identity
- persisted command evidence
- timeout
- bounded output
- safe cancellation
- structured cancellation/truncation evidence
- live command events
- operator command-history UI

### R0.2 - Runtime and Preview Operations

Required capabilities:
- runtime/machine registry
- heartbeat/offline truth
- Docker/devcontainer runtime control
- preview start/stop lifecycle
- preview-to-command/run linkage
- recovery behavior

### R0.3 - Workflow and Agent Operations

Required capabilities:
- task/workflow model
- PLAN/BUILD/VERIFY/GATE/MERGE/CLOSE lifecycle
- handoff ingestion
- verifier evidence
- approval gates
- agent/runtime assignment
- prompt packets
- no-fake-progress enforcement

### R0.4 - Production Security and Deployment

Required capabilities:
- authentication proof
- authorization/RBAC
- audit event completeness
- migration replay/rollback
- backup/restore
- secrets/config management
- observability
- deployment/upgrade/recovery runbooks
- CI merge enforcement

### R0.5 - Integrations and Later Expansion

Roadmap only:
- GitHub issue/PR/action bridge
- Tailscale-aware connectivity
- Obsidian-style memory integration
- advanced agent metrics
- IDE/file exploration
- full terminal only after security maturity

## Dependency Graph

Approved runner + persisted evidence
-> timeout/output guardrails
-> safe cancellation
-> structured safety evidence
-> live run event UX/history
-> runtime and preview lifecycle
-> workflow/agent governance
-> production deployment controls
-> integrations and advanced terminal/IDE capabilities

## Slice Registry

Completed:
- `COMMANDDECK-COMMAND-RUNNER-EXECUTION-GUARDRAILS-015`
  - objective: bounded stdout/stderr capture + deterministic truncation marker + timeout compatibility
  - branch/commit: `feature/commanddeck-command-runner-execution-guardrails-015` / `40f029196c98be3237fc0891eaee9542a0b5e8a8`
  - acceptance gate: focused Codex acceptance gate (Myles-authorized) passed and merged into `main`
  - release track: `R0.1`
- `COMMANDDECK-COMMAND-RUN-SAFE-CANCELLATION-VERTICAL-SLICE-016`
  - objective: safe run-ID-scoped cancellation endpoint + daemon cancellation path + UI action
  - branch/commit: `feature/commanddeck-command-run-safe-cancellation-016` / `d8fcee768ea86c10e068e19df6b9669d05b647e4` (merged)
  - acceptance gate: focused Codex acceptance gate passed, including race/leak correction
  - release track: `R0.1`
- `COMMANDDECK-CONTROL-PLANE-COMMERCIAL-SPRINT-017`
  - objective: structured truncation and cancellation-request evidence persistence/API/UI
  - branch/commit: `feature/commanddeck-command-run-structured-evidence-017` / `a7e4a3b773fc073fbf012031ba8b384765a62312` (merged)
  - acceptance gate: `CODEX_AUTHORIZED_ACCEPTANCE_GATE` passed (migration/sqlc/backend/frontend/full-suite/build/health)
  - release track: `R0.1`
- `COMMANDDECK-COMMAND-RUN-LIVE-EVENTS-018`
  - objective: workspace-scoped live command-run lifecycle events into CommandDeck UI with safe fallback polling
  - branch/commit: `feature/commanddeck-command-run-live-events-018` / `45bf5b820df745306cb8cca9be2385bee4a4519e` (merged)
  - acceptance gate: `CODEX_AUTHORIZED_ACCEPTANCE_GATE` passed (doctor/backend/frontend/full-suite/build/health)
  - release track: `R0.1`
- `COMMANDDECK-RUNTIME-HEALTH-HEARTBEAT-OFFLINE-019`
  - objective: truthful runtime heartbeat/offline status board with server-derived health status
  - branch/commit: `feature/commanddeck-runtime-health-heartbeat-offline-019` / `5b154fc8af552f33a6afefd349c44f94191275c9` (merged)
  - acceptance gate: `CODEX_AUTHORIZED_ACCEPTANCE_GATE` passed (doctor/backend/frontend/full-suite/build/health)
  - release track: `R0.2`
- `COMMANDDECK-PREVIEW-COMMAND-PROVENANCE-020`
  - objective: trusted preview-to-command provenance linkage where evidence is server-provable
  - branch/commit: `feature/commanddeck-preview-command-provenance-020` / `DEFERRED_NO_SAFE_LINKAGE_PREREQUISITE`
  - acceptance gate: `CODEX_AUTHORIZED_ACCEPTANCE_GATE` no-go for implementation (safe provenance source not present)
  - release track: `R0.2`
  - blocker:
    - current trusted preview write path (`POST /api/commandrunner/previews/self-hosted/sync`) intentionally persists `runtime_id=NULL` and `command_run_id=NULL`
    - command executor allowlist currently permits only git inspection commands, so no trusted preview-start command-run signal exists
  - required dependency:
    - add a trusted server-side preview registration path sourced from daemon/runtime-authenticated execution evidence carrying `command_run_id` + `runtime_id`
- `COMMANDDECK-PREVIEW-RUNTIME-AUTHENTICATED-REGISTRATION-020A`
  - objective: trusted daemon/runtime-authenticated preview registration that persists verified runtime provenance without unsafe command provenance inference
  - branch/commit: `feature/commanddeck-preview-runtime-authenticated-registration-020a` / `8127f4b6` (merged; main at `96a539b99dc49db23167a3aa30786a9d4738d09f`)
  - acceptance gate: `CODEX_AUTHORIZED_ACCEPTANCE_GATE` passed (doctor/backend/frontend/full-suite/build/health)
  - release track: `R0.2`
  - delivered capability:
    - daemon-token-only preview report route: `POST /api/daemon/runtimes/{runtimeId}/previews/report`
    - server-derived runtime provenance (`runtime_id`) from authenticated daemon/runtime context
    - payload spoofing rejection for mismatched `runtime_id`
    - explicit preservation of `command_run_id = NULL` in this path
    - UI distinction between verified runtime provenance and unproven preview provenance
  - remaining dependency:
    - trusted server-issued preview lifecycle correlation is still required before any command-run provenance linkage
- `COMMANDDECK-PREVIEW-LIFECYCLE-RECOVERY-CONTROL-021`
  - objective: trusted preview lifecycle status + non-destructive retirement/recovery controls for stale/offline runtime-reported previews
  - branch/commit: `feature/commanddeck-preview-lifecycle-recovery-control-021` / `6f8098ec8f9baf5e942b8fbce4067bb183335496` (merged)
  - acceptance gate: `CODEX_AUTHORIZED_ACCEPTANCE_GATE` passed (doctor/backend/sqlc/migration replay/frontend/full-suite/build/health)
  - release track: `R0.2`
  - delivered capability:
    - migration `089` retirement metadata (`retired_at`, `retired_by_type`, `retired_by_id`)
    - authenticated workspace-scoped retire endpoint: `POST /api/commandrunner/previews/{previewId}/retire`
    - active preview listing excludes retired entries
    - trusted runtime re-report reactivates retired preview record deterministically
    - UI lifecycle rendering and retire action for stale/offline/runtime-disconnected previews
  - remaining dependency:
    - command-run provenance linkage still requires trusted server-issued preview operation correlation
- `COMMANDDECK-APPROVED-PREVIEW-LAUNCH-PROVENANCE-022`
  - objective: add bounded approved preview lifecycle operation with server-issued correlation for safe runtime/command provenance linkage
  - branch/commit: `feature/commanddeck-approved-preview-launch-provenance-022` / `DEFERRED_ARCHITECTURE_PREREQUISITE_NOT_YET_PRESENT`
  - acceptance gate: `CODEX_AUTHORIZED_ACCEPTANCE_GATE` no-go for implementation (no trusted server-issued preview launch correlation path exists yet)
  - release track: `R0.2`
  - blocker:
    - preview reports can prove runtime provenance, but there is still no bounded approved preview-start operation that emits server-owned correlation evidence
    - linking `command_run_id` from runtime-reported preview data would still require unsafe inference
- `COMMANDDECK-WORKFLOW-EXECUTION-RECORD-FOUNDATION-022A`
  - objective: introduce workspace-scoped workflow execution records with real lifecycle states and optional command-run evidence association
  - branch/commit: `feature/commanddeck-workflow-execution-record-foundation-022a` / `5f93bc0a` (merged)
  - acceptance gate: `CODEX_AUTHORIZED_ACCEPTANCE_GATE` passed (doctor/backend/sqlc/migration replay/frontend/full-suite/build/health)
  - release track: `R0.3`
  - delivered capability:
    - migration `090` creates `command_workflow_execution` with lifecycle statuses (`planned`, `running`, `needs_review`, `completed`, `failed`, `cancelled`)
    - workspace-scoped API endpoints:
      - `GET /api/commandrunner/workflows`
      - `POST /api/commandrunner/workflows`
      - `GET /api/commandrunner/workflows/{workflowId}`
      - `PATCH /api/commandrunner/workflows/{workflowId}/status`
    - command-run evidence linkage is explicit and workspace-validated (`command_run_id` optional, cross-workspace association rejected)
    - CommandDeck UI adds a workflow execution panel with truthful empty/data states, record creation, and bounded lifecycle progression controls

Next selected slice:
- `COMMANDDECK-APPROVED-PREVIEW-LAUNCH-PROVENANCE-022B`
  - objective: add one bounded approved preview lifecycle operation with server-issued correlation so preview registration can safely attach verified `command_run_id`
  - dependency: `COMMANDDECK-PREVIEW-LIFECYCLE-RECOVERY-CONTROL-021` and `COMMANDDECK-WORKFLOW-EXECUTION-RECORD-FOUNDATION-022A`
  - release track: `R0.2`

Backlog candidates:
- command-run live events stream + richer operator history filtering (`R0.1`)
- runtime/preview lifecycle controls and recovery semantics (`R0.2`)
- workflow enforcement path BUILD->VERIFY->GATE->MERGE (`R0.3`)

## Commercial Readiness Definition

CommandDeck is not production-ready until at least:
- authentication and authorization are proven
- command execution audit evidence is complete
- migration replay/rollback is proven
- CI is reliably green and merge-gating
- deployment/recovery/backup path exists
- no fake operational status exists
- runtime/preview truth is verified
- critical operator workflows have real end-to-end proof
