# COMMANDDECK-PREVIEW-COMMAND-PROVENANCE-020

## Task ID
`COMMANDDECK-PREVIEW-COMMAND-PROVENANCE-020`

## Branch
`feature/commanddeck-preview-command-provenance-020`

## Base
`origin/main` at `5b154fc8af552f33a6afefd349c44f94191275c9` (runtime heartbeat/offline truth `019` merged)

## Objective
Link previews to trusted command-run/runtime evidence only when provenance is provable by server-owned signals.

## Inspection Result
Implementation deferred for safety. Current architecture does not provide a trusted preview-start execution signal that can prove `preview -> command_run -> runtime`.

### Evidence
- `server/internal/handler/previewregistry.go`
  - `HandleCommandDeckPreviewSelfHostedSync` is the only trusted preview write path in this slice.
  - `upsertSelfHostedPreviewRecord` explicitly writes:
    - `RuntimeID: pgtype.UUID{}` (null)
    - `CommandRunID: pgtype.UUID{}` (null)
- `server/internal/handler/previewregistry_test.go`
  - `TestHandleCommandDeckPreviewSelfHostedSyncDoesNotAssignUnprovenRuntime` asserts runtime/machine provenance remains unlinked for self-hosted preview sync.
- `server/internal/daemon/cmdexec/executor.go`
  - command allowlist remains git-inspection only (`git status`, `git branch --show-current`, `git rev-parse HEAD`, `git diff --stat`), so there is no approved preview-start command that could generate trustworthy linkage evidence.

## Safety Decision
- `CODEX_AUTHORIZED_ACCEPTANCE_GATE`: no-go for direct `020` linkage implementation in this sprint.
- Reason: any linkage from browser-submitted sync input would be inferred/untrusted provenance and would violate the no-fake/no-unsafe-inference rules.

## Required Prerequisite (Next Branch Spec)
`feature/commanddeck-preview-provenance-prereq-020a`

Deliver a trusted server-side preview registration path sourced from daemon/runtime-authenticated command execution evidence, including:
- verified `command_run_id`
- verified `runtime_id`
- workspace scoping checks
- persistence into `preview_registry` without browser-asserted provenance

Only after this prerequisite lands should `020` persist/display preview-to-command provenance links.
