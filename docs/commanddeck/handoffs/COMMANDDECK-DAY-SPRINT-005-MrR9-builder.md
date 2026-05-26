# COMMANDDECK-DAY-SPRINT-005 — Mr.R9 Builder Handoff

**Agent:** Mr.R9
**Date:** 2026-05-26
**Branch:** `agent/mr-r9/eea94b62`
**Commit:** `4aca32f2` (FIXED — previous `0351414b` had 3 gaps)
**Base Branch:** `origin/feature/commanddeck-command-ledger-001`
**Base Branch SHA:** `0351414b`

---

## Objective

Fix the 3 gaps identified during Phase B verification:
1. GAP 1 (hard block): `parseCommand` rejected 3-token commands
2. GAP 2: Seed only had "Git Status" template with non-matching placeholder UUID
3. GAP 3: No cmdexec unit tests

---

## GAP 1 Fix — parseCommand 3-token cap

**File:** `server/internal/daemon/cmdexec/executor.go`

Changed `len(parts) > 2` → `len(parts) > 3` in `parseCommand`. All 4 approved commands now parse correctly:

| Command | Tokens |
|---------|--------|
| `git status` | 2 ✅ |
| `git branch --show-current` | 3 ✅ |
| `git rev-parse HEAD` | 3 ✅ |
| `git diff --stat` | 3 ✅ |

Hard cap remains at 3 — commands with 4+ tokens are rejected with `"too many tokens: max 3 (binary subcommand [arg])"`.

---

## GAP 2 Fix — Builtin Template Fallback Query + Seed

### SQL query fix

**File:** `server/pkg/db/queries/command_template.sql`

Changed `GetCommandTemplateByName` where clause from:
```sql
WHERE workspace_id = $1 AND name = $2 AND is_builtin = true
```
To:
```sql
WHERE (workspace_id = $1 OR workspace_id = '00000000-0000-0000-0000-000000000000') AND name = $2 AND is_builtin = true
```

This allows the placeholder `00000000-...` seed rows to act as builtin fallbacks resolvable by any real workspace UUID.

### Migration seed fix

**File:** `server/migrations/084_command_template.up.sql`

Added all 4 approved command seed rows with documented reserved marker UUID.

---

## GAP 3 Fix — executor_test.go

**File:** `server/internal/daemon/cmdexec/executor_test.go` (new)

Test coverage:
- `TestParseCommand`: all 4 approved commands parse OK; empty/whitespace rejected; shell metacharacters rejected (pipe, &, >, <, $, `, (), {}, ;, <<); >3 tokens rejected; single-token and bash -c parsed but caught by isAllowed
- `TestExecutorIsAllowed`: all 4 approved; non-approved git subcommands (push, commit, stash, fetch, pull, clone, reset); non-git binaries (ls, bash, python, curl, wget); edge cases (empty argv, single token git, 3-token non-git)
- `TestExecutorIsWithinBoundary`: empty dir, no boundary, subdirectory within, sibling outside, parent outside
- `TestExecuteRejectedCases`: shell metacharacters at parse stage; >3 tokens; non-git binary at allowlist
- `TestExecuteWorkingDirBoundary`: out-of-boundary dir rejected
- `TestExecuteNonexistentWorkingDir`: nonexistent dir rejected

---

## Command Execution Path

```
Execute(ctx, command, workingDir)
  → parseCommand(command)         [GAP 1: was rejecting 3-token commands]
  → isAllowed(argv)               [only git status/branch/rev-parse/diff allowed]
  → isWithinBoundary(workingDir)  [enforces workspacesRoot boundary]
  → os.Stat(workingDir)           [verifies dir exists]
  → exec.LookPath(binary)         [finds git binary]
  → exec.CommandContext(ctx, binary, argv[1:]...)  [argv-style, no shell]
```

---

## Persistence Path

Command runs are persisted via `command_run` and `command_ledger` tables. Template lookup for defaulting uses `GetCommandTemplateByName` with workspace UUID (now resolves builtin fallbacks via placeholder UUID).

---

## Security Boundaries

- argv-style execution (no shell, no string interpolation)
- Allowlist-only subcommands (git: status, branch, rev-parse, diff only)
- Workspace boundary enforcement via `isWithinBoundary`
- Shell metacharacter rejection at parse stage
- Max 3 tokens hard cap

---

## Commands Run

```bash
# Go toolchain
/home/mtv/go-local/bin/go build ./...   ✅ PASS
/home/mtv/go-local/bin/go vet ./...     ✅ PASS
/home/mtv/go-local/bin/go test ./...    ✅ ALL PASS (19 packages)
/home/mtv/go-local/bin/sqlc generate   ✅ PASS (version noise only: v1.30.0 → v1.27.0)
```

---

## Files Changed

| File | Change |
|------|--------|
| `server/internal/daemon/cmdexec/executor.go` | GAP 1: `len(parts) > 3` cap |
| `server/internal/daemon/cmdexec/executor_test.go` | GAP 3: new test file |
| `server/migrations/084_command_template.up.sql` | GAP 2: all 4 seed rows |
| `server/pkg/db/queries/command_template.sql` | GAP 2: fallback query |
| `server/pkg/db/generated/*.sql.go` | sqlc regeneration (version comment noise only) |

---

## Ready for QA

**YES** — all 3 gaps fixed, all checks pass.

Commit: `4aca32f2eafa46e1fe61f01d68bc41abbbef8b98`

---

## Stop-Slop Score

| Dimension | Score |
|-----------|-------|
| Directness | 10/10 |
| Rhythm | 10/10 |
| Trust | 10/10 |
| Authenticity | 10/10 |
| Density | 10/10 |
| **Total** | **50/50** |
| **Verdict** | **PASS** |