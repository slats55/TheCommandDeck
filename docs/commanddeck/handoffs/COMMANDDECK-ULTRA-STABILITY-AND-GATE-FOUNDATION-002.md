# COMMANDDECK-ULTRA-STABILITY-AND-GATE-FOUNDATION-002 REPORT

_Date: 2026-06-04 · Branch: `claude/commanddeck-ultra-stability-gate-foundation-002`_

## Final Status

**COMPLETE_READY_FOR_REVIEW**

All six slices landed with evidence and per-slice commits. Two items were
deliberately scoped out (self-hosting docs, clipboard) and are recorded below as
verified follow-ups — neither blocks review.

## Executive Summary

The local-dev bootstrap bug is fully closed and now hard to reintroduce. The
root cause was Docker Compose file precedence: the repo ships both `compose.yml`
(production "commanddeck" stack) and `docker-compose.yml` (local-dev "multica"
stack). A bare `docker compose` auto-selects `compose.yml`, which has **no
`postgres` service**, so every bare `docker compose … postgres` for local dev
failed with `no such service: postgres`. That was fixed in the bootstrap script,
the Makefile, and the local-dev docs — and the developer `doctor` now detects
the ambiguity explicitly so the next person sees the cause in one command.

The noisy local preview was diagnosed against a live server: the `401`s on
`/api/me` and `/api/workspaces` are the **expected** unauthenticated state
(the app correctly redirects to `/login`), they were just logged as scary red
errors. That logging is now right-sized without weakening auth.

Two real upgrade foundations shipped: a much stronger deterministic local
`doctor` (with safe `--fix` and machine-readable `--json`), and a read-only
`repo-impact` classifier that maps a branch's changes to subsystems and risk
flags — the first concrete input for the next sprint's verification gate.

## Starting State

- Branch at start: `claude/happy-hypatia-e5f72f` (harness worktree branch).
- HEAD at start: `1efd6d0a` (= `origin/main`; PR #6 merge).
- Dirty files at start: none — clean working tree.
- Prior uncommitted fix detected: **no.** The previous task
  (`COMMANDDECK-DEV-BOOTSTRAP-COMPOSE-FIX-001`) described an uncommitted fix and
  a handoff doc, but neither was present in this worktree. The fix was
  re-derived from first principles and this time committed.
- Unrelated dirty files: none. Safe to proceed.
- Preview status: an external worktree's preview was live at `http://localhost:18942`
  (backend) during diagnosis; it returned `200` on `/health` and the expected
  `401`s on protected endpoints. It later went down (external process, not owned
  by this sprint).

Work was done on a dedicated branch `claude/commanddeck-ultra-stability-gate-foundation-002`.

## Slice Results

### Slice 0 — Baseline
- Objective: protect uncommitted work, verify baseline.
- Result: working tree clean at `origin/main`; prior fix absent. Safe to proceed.

### Slice 1 — Complete local bootstrap Compose fix
- Objective: pin every implicit local-dev `postgres` compose command to `docker-compose.yml`.
- Files: `scripts/ensure-postgres.sh`, `Makefile`, `CONTRIBUTING.md`, `apps/docs/content/docs/developers/contributing.zh.mdx`.
- Commit: **`a709bbc0`**.
- Verification: `bash -n` OK; `docker compose config --services` proves bare → `commanddeck-*` (no `postgres`) while `-f docker-compose.yml` → `postgres`; `ensure-postgres.sh .env.worktree` created the worktree DB end-to-end; `db-up` recipe command exit 0.
- Pass/fail: **PASS.**
- Risks: low. Production (`compose.yml`) and self-host (`docker-compose.selfhost.yml`) paths untouched.

### Slice 2 — Auth/preview diagnosis
- Objective: classify the `401`s, the `cookie auth init failed` log, and the clipboard error.
- Result (live-evidenced against `:18942`): `401`s are expected unauthenticated state; the noise is purely client-side error-level logging. See "Bugs Diagnosed".
- Commit: none (diagnosis only).
- Pass/fail: **PASS.**

### Slice 3 — Auth/preview clarity fix
- Objective: smallest safe fix from Slice 2.
- Files: `packages/core/api/client.ts`, `packages/core/platform/auth-initializer.tsx`, `packages/core/api/client.test.ts`.
- Commit: **`d73dc9bb`**.
- Verification: `@multica/core` typecheck clean; full core suite 234/234 (incl. 2 new tests: `401`→warn, `500`→error).
- Pass/fail: **PASS.**
- Risks: low. No auth behavior change — the `401`s still occur and still drive the `/login` redirect; only log levels changed.

### Slice 4 — Deterministic local doctor
- Objective: extend the existing doctor into a worktree-aware local-dev diagnostic with safe repair.
- Files: `scripts/doctor.sh`, `scripts/doctor.ps1`.
- Commit: **`b7715d0a`**.
- Verification: `bash -n` OK; ran report / `--fix` / `--json` on bash and report / `-Json` on PowerShell; JSON parses in both; DB password masked (`****`), no raw credential present.
- Pass/fail: **PASS.**
- Risks: low. `--fix` only runs existing idempotent scripts; never resets the DB, never auto-runs migrations, never prints secrets.

### Slice 5 — Repo impact classifier
- Objective: read-only branch-impact report (subsystems + risk flags) as the gate foundation.
- Files: `server/internal/repoimpact/{repoimpact.go,repoimpact_test.go}`, `server/cmd/repo-impact/main.go`, `Makefile` (`make repo-impact`).
- Commits: **`21d8d5df`** + **`3c16e68c`** (header-anchored generated-code detection, fixing a self-referential false positive found during final verification).
- Verification: `gofmt` clean; `go vet` clean; `go build ./...` whole module OK; `go test ./internal/repoimpact/` green; CLI classifies this branch correctly (human + valid `--json`).
- Pass/fail: **PASS.**
- Risks: low. Stdlib only, no DB writes, git-only exec with fixed args.

### Slice 6 — Final verification + report
- Objective: full verification matrix + this handoff.
- Result: see Verification Matrix. This document committed as the final step.

## Bugs Fixed

1. **Docker Compose precedence (local-dev DB bootstrap).** Bare `docker compose … postgres` selected `compose.yml` (no `postgres` service) → `no such service: postgres`. Pinned to `docker-compose.yml` in `ensure-postgres.sh` (script-relative), the `Makefile` `COMPOSE` var (drives `db-up`/`db-down`/`db-reset` only), and the local-dev docs (`list databases`, `wipe local data`). Commit `a709bbc0`.
2. **Makefile local-DB commands using the wrong Compose file** — the follow-up the prior task flagged out of scope. Fixed in `a709bbc0` (same root cause).
3. **Expected unauthenticated `401`s logged as errors.** The API client logged every non-404 error response at `error` (so `← 401 /api/me` looked like a fault), and `AuthInitializer` logged the expected probe rejection as `cookie auth init failed` at `error`. Now `401` logs at `warn` (client) / `debug` (initializer); genuine failures still log at `error`. Commit `d73dc9bb`.
4. **Doctor self-referential generated-code false positive** (found in final verification). Commit `3c16e68c`.

## Bugs Diagnosed But Not Fixed

| Symptom | Root cause | Verdict | Action |
| --- | --- | --- | --- |
| `401 GET /api/me` (no auth) | `AuthInitializer` session probe before login; backend `auth.go` returns 401 `missing authorization`. Verified live. | **Expected** | Logging right-sized in Slice 3; no auth change. |
| `401 GET /api/workspaces` (no auth) | Second call in the same probe. | **Expected** | Same. |
| `[auth] cookie auth init failed: missing authorization` | `auth-initializer.tsx:97` logged expected `401` at `error`. | **Expected state, mislabeled** | Fixed (Slice 3). |
| `NotAllowedError: writeText … permission denied` | App-owned copy actions (`navigator.clipboard.writeText`) rejected because the preview iframe lacks `clipboard-write` Permissions-Policy. Environmental, not a product fault; some call sites already guard (e.g. `cli-section.tsx`), others don't. | **Environmental / secondary** | Not fixed — would need a shared safe-copy helper across ~15 call sites (out of scope). Recommended as a follow-up. |
| Daemon needs CLI auth/session before connecting | `daemon_auth.go` requires a session; the daemon isn't started in a web preview. | **Expected** | Documented; not a bug. The dev login path is in `docs/commanddeck/runbooks/LOCAL-DOGFOOD-ACCESS.md`. |
| Self-hosting docs `docker compose up -d postgres` | Bare command mis-selects `compose.yml`; the self-host `postgres` service is in `docker-compose.selfhost.yml`. | **Real, but different subsystem** | Out of local-dev scope; flagged as a follow-up task chip. Sites: `apps/docs/content/docs/getting-started/self-hosting.zh.mdx:279,311`, `SELF_HOSTING_ADVANCED.md:160`. |

The intended local login path is documented and confirmed: open `/login` → dev
sign-in (email + dev verification code, printed to API logs when no email
provider is set). `dev_auth_enabled: true` is exposed by the public
`/api/config` (boolean only — the code is never returned). The dashboard guard
(`use-dashboard-guard.ts`) redirects unauthenticated users to `/login`, so the
unauthenticated UX is already clean.

## New Capabilities Added

- **`doctor --fix` / `-Fix`** — safe, idempotent local repair (generate
  `.env.worktree`, ensure postgres). Report-only by default.
- **`doctor --json` / `-Json`** — machine-readable diagnostics for the gate runner.
- **`repo-impact` CLI + `make repo-impact`** — deterministic branch-impact report
  (subsystems + risk flags), human or `--json`.

## Self-Healing / Doctor Notes

**How to run:** `pnpm doctor` (bash) or `pnpm doctor:ps` (PowerShell); add
`--fix`/`-Fix` or `--json`/`-Json`.

**What it checks (report-only by default):** git (version, branch, dirty);
worktree vs main checkout; node / pnpm / **go** / docker; active env file
(`.env`, else `.env.worktree`); **Docker Compose ambiguity** (explains that
local-dev DB must use `-f docker-compose.yml`); the `postgres` service in
`docker-compose.yml`; DB container running / accepting connections / target DB
exists; `DATABASE_URL` (password masked); env-aware preview probes
(`PORT`/`FRONTEND_PORT`); a note that desktop/daemon are optional.

**What it repairs (only with `--fix`):** generates `.env.worktree` via the
existing init script; ensures postgres via `ensure-postgres.sh`. Both idempotent.

**What it refuses:** never resets the DB, never auto-runs migrations (it prints
the command), never mutates source, never prints secrets.

**Secret proof:** `db.url` is emitted as `postgres://multica:****@…`; validated
in both bash and PowerShell that the masked form is present and the raw
credential (`multica:multica@`) is absent from `--json` output. Exits non-zero
only on hard failures.

## Repo Impact Classifier Notes

**How to run:** `make repo-impact` (or `cd server && go run ./cmd/repo-impact
[--base origin/main] [--head HEAD] [--json]`).

**What it outputs:** `baseRef`, `headRef`, `changedFiles`, `subsystems`
(partition into `frontend / backend / daemon_runtime / database_migrations /
docs / infra_docker / scripts_devtools / tests / github_ci / security_auth /
command_exec / unknown`), `riskFlags` (`{name, severity, evidence}`), and a
human `summary`. Risk flags: migrations / auth-security / command-execution
(high); runtime-daemon / docker-compose / generated-code / dependencies
(medium); large-diff / docs-only / tests-missing (low). `docker_compose` and
`generated_code` also match by file content, so a Makefile/script that wields
`docker compose` is flagged even though it is not itself a compose file.

**Detected on this branch:** 12 changed files across `backend, docs, frontend,
scripts_devtools, security_auth, tests`; risk flags `auth_security_touched`
(high, evidence: `auth-initializer.tsx`) and `docker_compose_touched` (medium,
evidence: `Makefile`, `ensure-postgres.sh`, both contributing docs, both doctor
scripts). `tests_missing_for_code_change` correctly suppressed because
`client.test.ts` accompanies the code changes.

**How it supports the gate:** the stable, ordered `--json` document is the
structured evidence a verification gate record can consume to block merge until
risk-appropriate checks pass.

## Verification Matrix

| Command | Result | Notes |
| --- | --- | --- |
| `bash -n scripts/ensure-postgres.sh` | PASS | Syntax OK |
| `docker compose config --services` (bare) | PASS | Proves bug: `commanddeck-*`, no `postgres` |
| `docker compose -f docker-compose.yml config --services` | PASS | `postgres` |
| `bash scripts/ensure-postgres.sh .env.worktree` | PASS | Created worktree DB end-to-end |
| `docker compose -f docker-compose.yml up -d postgres` (db-up recipe) | PASS | exit 0 |
| `pnpm --filter @multica/core typecheck` | PASS | Clean |
| `pnpm --filter @multica/core test` | PASS | 234/234 (28 files) |
| `bash -n scripts/doctor.sh` | PASS | Syntax OK |
| `bash scripts/doctor.sh` (report) | PASS | exit 0, 0 failures |
| `bash scripts/doctor.sh --json` | PASS | Valid JSON (20 checks), masked secret |
| `bash scripts/doctor.sh --fix` | PASS | Idempotent; migrations only printed |
| `powershell -File scripts/doctor.ps1` | PASS | Parity output, masked secret |
| `powershell -File scripts/doctor.ps1 -Json` | PASS | Valid JSON, masked secret |
| `gofmt -l` (new Go) | PASS | Clean |
| `go vet ./internal/repoimpact ./cmd/repo-impact` | PASS | Clean |
| `go build ./...` (server) | PASS | Whole module compiles |
| `go test ./internal/repoimpact/` | PASS | All table tests green |
| `go run ./cmd/repo-impact` (+ `--json`) | PASS | Correctly classifies this branch |
| grep local-dev docs for bare compose+postgres | PASS | None remain |
| `/api/me`, `/api/workspaces` unauth (live) | PASS | `401 missing authorization` (expected) |

**Not run, with reason:** full `pnpm typecheck` / `pnpm test` across all
packages and `go test ./...` (server) and Playwright E2E were not run — they
require the full monorepo build / a migrated DB / a running stack, and the
changes are localized to `@multica/core` (typechecked + fully tested here) and a
new self-contained Go package (built + tested here). `make check` was not run
per the "verify only what's asked" convention; targeted checks above cover the
touched surface.

## Security Review

- No fake data added — every panel/status remains backed by real state.
- No auth disabled — `401`s still occur and still drive the `/login` redirect;
  only client log levels changed.
- No secrets printed or committed — `doctor` masks `DATABASE_URL`; verified the
  raw credential is absent from `--json`. `.env.worktree` is gitignored and was
  never staged.
- No destructive DB reset — `doctor --fix` refuses resets; no `down -v` run.
- No raw browser terminal / no mock dashboards added.
- No production stack targeted — `compose.yml`, `compose.dev.yml`,
  `compose.prod.yml`, and the self-host compose files were not modified.
- No broad unrelated rewrite — 5 atomic commits, each scoped to one slice.

## Git State

- Branch: `claude/commanddeck-ultra-stability-gate-foundation-002`
- HEAD: `3c16e68c`
- Commits created (oldest first):
  - `a709bbc0` fix(dev): pin local postgres compose commands to docker-compose.yml
  - `d73dc9bb` fix(auth): log expected unauthenticated 401s at debug/warn, not error
  - `b7715d0a` feat(dev): deterministic local doctor with safe --fix and --json
  - `21d8d5df` feat(dev): add repo impact classifier
  - `3c16e68c` fix(dev): anchor generated-code detection to the file header
  - _(+ this report, committed as the final docs step)_
- Files changed vs `origin/main`: 12 code/doc files + this report.
- Untracked: none (besides the gitignored `.env.worktree` generated for testing).
- Staged: none after each commit.
- Pushed: **no** — branch is local. Push only on explicit authorization.

## Recommended Next Task

**COMMANDDECK-VERIFICATION-GATE-RECORD-001**

Use `repo-impact --json` output plus command-run evidence to produce a
no-mistakes-style verification gate record that blocks merge approval until the
risk-appropriate checks pass (e.g. `migrations_touched` → require migration
test + down-migration; `auth_security_touched` → require the auth test suite;
`docker_compose_touched` → require `doctor --json` clean). Append records to the
existing command/ledger tables (084–090) rather than introducing new storage.
