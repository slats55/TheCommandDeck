## TASK_ID

`COMMANDDECK-COMMAND-RUNNER-EXECUTION-GUARDRAILS-015`

## Baseline

- Repo root: `C:\Users\mtval\PycharmProjects\TheCommandDeck`
- Starting `origin/main`: `759943234c268cfc10fbd75d26adf9a9f0ac3bda`
- Prior docs audit branch fast-gated and merged first:
  - `origin/chore/commanddeck-authenticated-smoke-production-readiness-audit-014`
  - commit: `f6b699e9b13d338a2d1edcc8e9c69dcd35f567af`
  - merge mode: fast-forward into `main`
  - resulting `origin/main` for feature base: `f6b699e9b13d338a2d1edcc8e9c69dcd35f567af`

## What Existed Before This Change

- Command execution entry point: `server/internal/daemon/cmdexec/Executor.Execute`
- Existing guardrails already present:
  - strict allowlist by exact argv key
  - shell metacharacter rejection during parse
  - workspace boundary enforcement on working directory
  - backend enforced timeout status path (`timeout`) using `context.WithTimeout`
- Missing/inadequate guardrail:
  - no bounded stdout/stderr capture in execution path (`strings.Builder` was unbounded)
  - no test proving output capture truncation behavior
  - timeout tests were not deterministic for deadline handling path

## Safeguards Implemented

1. Bounded stdout/stderr capture in daemon executor:
   - Added hard caps:
     - `MaxStdoutBytes = 64 * 1024`
     - `MaxStderrBytes = 64 * 1024`
   - Added truncation marker:
     - `"[output truncated by CommandDeck safety limit]"`
   - Captured output now uses a capped writer that preserves up to limit and marks truncation truthfully.

2. Timeout execution behavior tightened and test-injectable:
   - Executor now keeps `maxDuration` field initialized from `MaxDuration`.
   - Daemon WS wrapper now uses `MaxDuration` constant (single source for timeout value instead of literal duplicate).
   - Timeout status remains `timeout`; when timeout occurs with empty stderr, a truthful timeout message is set:
     - `"command timed out before completion"`

3. Deterministic execution seams for tests:
   - Added internal `runFn` function field on Executor for controlled test injection.
   - Production path still uses real `runCommand`.

## Output and Timeout Policy

- Timeout policy:
  - backend-owned duration cap: `30s` (`MaxDuration`)
  - enforced in command execution layer via context timeout
- Output policy:
  - stdout cap: 64 KiB
  - stderr cap: 64 KiB
  - overflow is truncated and explicitly marked with the safety marker

## Files Changed

- `server/internal/daemon/cmdexec/executor.go`
- `server/internal/daemon/cmdexec/daemon.go`
- `server/internal/daemon/cmdexec/executor_test.go`

## Tests Added / Updated

Added in `executor_test.go`:

- `TestExecuteMarksTimeoutWhenRunnerDeadlineExceeded`
- `TestRunCommandBoundsOutputAndMarksTruncation`
- `TestExecuteAppendsTruncationMarker`
- helper: `installFakeAllowedCommand`
- helper: `requireGit` (skips git-dependent tests when git is unavailable in runtime)
- helper process: `TestHelperLargeOutputProcess`

Existing behavior remained covered:

- allowlist rejection tests
- workspace boundary rejection tests
- approved command flow tests (when git available)

## Verification Commands Run and Results

1. `powershell -ExecutionPolicy Bypass -File scripts/doctor.ps1`
   - PASS
   - Warnings only: dirty worktree during development, no upstream for local feature branch (expected while building)

2. `docker run --rm -v "${PWD}/server:/src" -w /src golang:1.26 /bin/sh -lc "export PATH=/usr/local/go/bin:$PATH; go test ./internal/daemon/cmdexec ./internal/handler ./cmd/server && go build ./cmd/server"`
   - PASS

3. `pnpm.cmd lint`
   - PASS with existing warnings (no new blocking lint errors)

4. `pnpm.cmd test`
   - PASS (existing non-blocking warnings remain)

5. `pnpm.cmd build`
   - PASS (existing non-blocking build warnings remain)

6. Local stack checks:
   - `http://localhost:8080/health` => `200`
   - `http://localhost:3000` => `200`
   - `http://localhost:3000/login` => `200`

## Security Analysis

- No arbitrary shell execution introduced.
- No allowlist broadening for production commands.
- Workspace boundary enforcement retained.
- Timeout remains backend-enforced.
- Runtime identity flow in command-runner handler unchanged.
- No preview-registry logic touched.
- No schema migration introduced.
- No secrets committed.

## Schema / API / UI Impact

- Database schema: unchanged.
- SQL/sqlc: unchanged.
- API contract: unchanged JSON shape for command run result.
- UI: unchanged in this slice (backend safety only).

## Known Remaining Gaps (Out of Scope Here)

- Structured persisted `output_truncated` / `timed_out_reason` flags are not added as dedicated DB columns; truncation truth is conveyed via output marker and timeout status path.
- Authenticated CommandDeck dashboard smoke proof remains a separate task path.
- Clean-db migration replay proof is separate from this executor hardening slice.

## Independent Verifier Instructions

1. Fetch branch:
   - `feature/commanddeck-command-runner-execution-guardrails-015`
2. Inspect diff scope:
   - only `server/internal/daemon/cmdexec/*` plus this handoff
3. Re-run core guardrail checks:
   - `docker run --rm -v "${PWD}/server:/src" -w /src golang:1.26 /bin/sh -lc "export PATH=/usr/local/go/bin:$PATH; go test ./internal/daemon/cmdexec ./internal/handler ./cmd/server && go build ./cmd/server"`
4. Confirm assertions:
   - timeout status path test passes
   - output truncation cap/marker tests pass
   - allowlist and workspace boundary tests still pass
5. Re-run workspace checks:
   - `pnpm.cmd lint`
   - `pnpm.cmd test`
   - `pnpm.cmd build`
6. GO conditions:
   - no scope drift beyond daemon executor hardening
   - no security boundary regressions
   - all checks above pass
7. NO-GO conditions:
   - any allowlist loosening
   - missing truncation marker on bounded overflow
   - timeout path not represented as `timeout`

