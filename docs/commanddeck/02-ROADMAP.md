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
  - branch/commit: `feature/commanddeck-command-run-live-events-018` / `PENDING_MERGE`
  - acceptance gate: `CODEX_AUTHORIZED_ACCEPTANCE_GATE` in progress on branch
  - release track: `R0.1`

Next selected slice:
- `COMMANDDECK-RUNTIME-HEALTH-HEARTBEAT-OFFLINE-019`
  - objective: truthful runtime heartbeat/offline status board (workspace-scoped)
  - dependency: merge and verify `018`
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
