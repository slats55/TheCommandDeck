# COMMANDDECK-CI-MERGE-EVIDENCE-HARDENING-023

- Task ID: `COMMANDDECK-CI-MERGE-EVIDENCE-HARDENING-023`
- Branch: `chore/commanddeck-ci-merge-evidence-hardening-023`
- Base: `origin/main` at `22cafdd143aca1318ec89ca62e02b768d696cb3d`
- Verification label: `CODEX_SELF_VERIFICATION`
- Acceptance label: `CODEX_AUTHORIZED_ACCEPTANCE_GATE`

## Objective

Harden checked-in CI merge evidence by adding deterministic SQLC drift checks and migration replay (`up -> down -> up`) in the backend GitHub Actions workflow.

## Files Changed

- `.github/workflows/ci.yml`

## Delivered Behavior

1. Added backend SQLC generated-code drift gate:
   - install `sqlc@v1.27.0`
   - run `sqlc generate`
   - fail workflow if `server/pkg/db/generated` changes
2. Strengthened migration verification:
   - changed from `migrate up` only
   - to `migrate up`, `migrate down`, `migrate up`

## Security/Scope Assertions

- No branch protection or repository settings changes.
- No deployment steps added.
- No secrets/tokens added or printed intentionally.
- No product runtime behavior changes.
- No command-execution surface changes.

## Verification Evidence

- Targeted workflow lint:
  - `docker run --rm -v "${PWD}:/repo" -w /repo rhysd/actionlint:1.7.8 .github/workflows/ci.yml` (pass)
- SQLC local equivalent:
  - `docker run --rm -v "${PWD}/server:/src" -w /src sqlc/sqlc:1.27.0 generate` (pass)
- Migration replay local equivalent (disposable DB):
  - `go run ./cmd/migrate up; go run ./cmd/migrate down; go run ./cmd/migrate up` (pass)
- Frontend CI command equivalence with real execution (cache bypass):
  - `pnpm.cmd exec turbo build typecheck lint test --filter='!@multica/docs' --force` (pass)
- Baseline frontend commands:
  - `pnpm.cmd lint` (pass)
  - `pnpm.cmd test` (pass)
  - `pnpm.cmd build` (pass)
- Baseline backend focused command:
  - `go test ./internal/daemon/cmdexec ./internal/daemonws ./internal/handler ./cmd/server && go build ./cmd/server` in Docker (pass)
- Doctor:
  - `powershell -ExecutionPolicy Bypass -File scripts/doctor.ps1` (pass)

## Known Limitations

- Full `go test ./...` in minimal `golang:1.26` container showed environment-sensitive failures:
  - `git` missing in container PATH for daemon repo tests
  - timeout flakes in some `server/pkg/agent` tests
- This does not invalidate the workflow YAML change, but remote CI proof is still required.
- Status for this slice: `CI_CONFIG_LANDED_PENDING_REMOTE_RUN_PROOF`.

## Next Recommended Task

`COMMANDDECK-PREVIEW-LAUNCH-CORRELATION-PREREQUISITE-022C`
