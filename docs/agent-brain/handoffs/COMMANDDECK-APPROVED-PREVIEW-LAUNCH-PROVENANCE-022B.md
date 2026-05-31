# COMMANDDECK-APPROVED-PREVIEW-LAUNCH-PROVENANCE-022B

- Task ID: `COMMANDDECK-APPROVED-PREVIEW-LAUNCH-PROVENANCE-022B`
- Status: `BLOCKED_NO_SAFE_IMPLEMENTATION`
- Verification label: `CODEX_SELF_VERIFICATION`
- Acceptance label: `CODEX_AUTHORIZED_ACCEPTANCE_GATE`

## Objective

Implement one bounded approved preview-launch lifecycle operation with server-issued trusted correlation so authenticated runtime preview reporting can safely persist verified `runtime_id` and verified `command_run_id`.

## Architecture Gate Result

`NO_GO_UNSAFE_TRUSTED_LAUNCH_PREREQUISITE_STILL_MISSING`

### Evidence from current code

- Command execution allowlist remains git-inspection only:
  - `git status`
  - `git branch --show-current`
  - `git rev-parse HEAD`
  - `git diff --stat`
- Trusted runtime preview reporting exists (`POST /api/daemon/runtimes/{runtimeId}/previews/report`) but persists `command_run_id` as null in the current trusted path.
- No server-owned preview-launch correlation record/model exists to validate runtime/workspace/operation binding and replay/expiry consumption.
- Any direct `command_run_id` linkage from runtime-reported preview metadata would still be inference and would violate provenance trust rules.

## Security Decision

- Did not introduce any preview-to-command linkage by inference.
- Did not broaden command execution allowlist.
- Did not add arbitrary shell/process capability.
- Preserved existing trustworthy behavior:
  - runtime-authenticated preview provenance
  - non-provenance-asserting self-hosted sync
  - explicit nullable command provenance where unproven

## Merge/Git Result

- No product-code branch was merged for `022B` because the architecture gate returned `NO-GO`.
- Follow-on slice selected: `COMMANDDECK-CI-MERGE-EVIDENCE-HARDENING-023`.

## Next Recommended Task

`COMMANDDECK-PREVIEW-LAUNCH-CORRELATION-PREREQUISITE-022C`

Build a bounded server-owned preview-launch lifecycle correlation model (workspace/runtime/operation/expiry/consumption) before retrying command provenance linkage.
