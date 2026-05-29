# COMMANDDECK-COMMAND-RUNNER-LEDGER-GATE-MERGE-007

Date: 2026-05-29
Branch: integration/commanddeck-command-runner-ledger-006

## Remote State

- origin/main: 8d1ac8b226104d452869cd96aaa83acfadb24c18
- origin/integration/commanddeck-command-runner-ledger-006 before this gate: 83c2b7fed7dd0d60282fa03e95a69ef0640e744f
- Ledger branch was not contained in origin/main at resume time.
- Last known commit 83c2b7fed7dd0d60282fa03e95a69ef0640e744f was the remote branch head; no newer remote checkpoint commits were found.

## Repairs Added During Gate

- Tightened daemon command execution allowlist to exact argv forms:
  - git status
  - git branch --show-current
  - git rev-parse HEAD
  - git diff --stat
- Added executor tests for approved builtins, rejected shell metacharacters, rejected arbitrary commands, rejected unapproved git flags, workspace escape rejection, and exit/stdout/stderr recording.
- Fixed daemon WebSocket hub delivery so command_run:execute frames are routed to clients watching the target runtime, not only daemon:task_available frames.
- Added daemon WebSocket hub coverage proving command_run:execute delivery.

## Verification Evidence

- Docker preview rebuilt and started with compose.yml + compose.dev.yml.
- Runtime migrations applied through:
  - 084_command_template
  - 085_command_run
  - 086_command_ledger
- API health probe returned 200 with {"status":"ok"}.
- Web preview returned 200.
- Login route returned 200.
- Login copy check: "Sign in to CommandDeck" present; "Sign in to Multica" absent.
- Authenticated command templates endpoint returned the built-in Git Status template.
- Real command runner flow was proven through live local API plus daemon WebSocket after the final Docker rebuild:
  - Workspace ID: 31e268d0-8f50-4cdc-91b2-acc4eceece88
  - Runtime ID: 9e78b4a8-baed-4a86-89a0-ce0320c6424f
  - Template: Git Status
  - Command: git status
  - Run ID: c1c7d1b6-033a-42a1-a6b1-5f3e858ddaab
  - Status: completed
  - Exit code: 0
  - Ledger rows for run: 1
  - Stdout first line: On branch integration/commanddeck-command-runner-ledger-006
- powershell -ExecutionPolicy Bypass -File scripts/doctor.ps1 passed with one dirty-tree warning for the intended gate edits and existing .junie/ directory.
- pnpm lint passed with existing warnings only.
- pnpm build passed.
- pnpm test passed.
- go test ./... passed through Docker using golang:1.26-alpine with git installed in the temporary container.
- go build ./... passed through Docker using golang:1.26-alpine with git installed in the temporary container.

## Tooling Notes

- Local Go was not installed on this workstation.
- Go verification used the official golang:1.26-alpine Docker image with git installed inside the temporary container.
- Local .env remained ignored and was not committed.
- No secret values are included in this document.
