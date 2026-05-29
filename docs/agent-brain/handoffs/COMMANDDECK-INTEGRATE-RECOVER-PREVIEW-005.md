# COMMANDDECK-INTEGRATE-RECOVER-PREVIEW-005

## Branches inspected
- `fix/commanddeck-local-workspace-boot-001`
- `fix/commanddeck-monorepo-gate-cleanup-001`
- `origin/chore/commanddeck-agent-brain-scaffold-002`
- `origin/chore/commanddeck-discovery-001`
- `origin/chore/commanddeck-build-gate-001`
- `origin/chore/commanddeck-repo-doctor-001`
- `origin/feature/commanddeck-command-ledger-001`
- `origin/feature/commanddeck-overnight-006r`
- `origin/feature/commanddeck-security-gate-001`
- `origin/feature/commanddeck-slice-001-git-status-runner`
- `origin/feature/commanddeck-slice-002-build-verify-next-command`
- `origin/feature/commanddeck-slice-003-branch-command`
- `origin/feature/commanddeck-slice-003-branch-command-v2`
- `origin/feature/commanddeck-slice-004-rev-parse-head`
- `origin/feature/commanddeck-slice-005-git-diff-stat`
- `origin/feature/commanddeck-ui-run-control-006`
- `origin/test/commanddeck-command-runner-tests-004`
- `origin/docs/commanddeck-template-runner-design-001`
- `origin/fix/commanddeck-local-workspace-boot-001`
- `origin/fix/commanddeck-monorepo-gate-cleanup-001`

## Branches preserved to origin
- `fix/commanddeck-local-workspace-boot-001` -> pushed to `origin/fix/commanddeck-local-workspace-boot-001` (`0 0` sync confirmed)
- `fix/commanddeck-monorepo-gate-cleanup-001` -> pushed to `origin/fix/commanddeck-monorepo-gate-cleanup-001` (`0 0` sync confirmed)

## Integrated branches
- `fix/commanddeck-monorepo-gate-cleanup-001` integrated onto `integration/commanddeck-preview-recovery-005` (includes the auth branding and monorepo gate cleanup changes)

## Branches not integrated and why
- `origin/chore/commanddeck-agent-brain-scaffold-002`: already in `origin/main` (ancestor confirmed), no re-merge required.
- `origin/chore/commanddeck-discovery-001`, `origin/chore/commanddeck-build-gate-001`, `origin/chore/commanddeck-repo-doctor-001`, `origin/feature/commanddeck-command-ledger-001`, `origin/feature/commanddeck-overnight-006r`, `origin/feature/commanddeck-security-gate-001`, `origin/feature/commanddeck-slice-*`, `origin/feature/commanddeck-ui-run-control-006`, `origin/test/commanddeck-command-runner-tests-004`, `origin/docs/commanddeck-template-runner-design-001`: diverged heavily from current `origin/main` and require separate targeted reconciliation before safe merge.

## Real code/config/files changed in this integration
- Desktop test/runtime gate fixes:
  - `apps/desktop/scripts/package.mjs`
  - `apps/desktop/scripts/package.test.mjs`
  - `apps/desktop/test/setup.ts`
  - `apps/desktop/vitest.config.ts`
- Docs build fix:
  - `apps/docs/content/docs/github-integration.mdx`
  - `apps/docs/content/docs/github-integration.zh.mdx`
- CommandDeck auth branding/login slice:
  - `apps/web/app/(auth)/login/page.test.tsx`
  - `apps/web/app/auth/callback/page.tsx`
  - `packages/views/auth/login-page.test.tsx`
  - `packages/views/locales/en/auth.json`
  - `packages/views/locales/zh-Hans/auth.json`
- CommandDeck state/runbook/handoff docs carried from validated branch work:
  - `docs/commanddeck/01-CURRENT-STATE.md`
  - `docs/commanddeck/02-ROADMAP.md`
  - `docs/commanddeck/07-KNOWN-RISKS.md`
  - `docs/commanddeck/runbooks/LOCAL-SELFHOST-PREVIEW.md`
  - `docs/commanddeck/handoffs/COMMANDDECK-LOCAL-WORKSPACE-BOOT-001-CODEX.md`
  - `docs/commanddeck/handoffs/COMMANDDECK-LOCAL-WORKSPACE-BOOT-001-COMMIT.md`
  - `docs/commanddeck/handoffs/COMMANDDECK-MONOREPO-GATE-CLEANUP-001-CODEX.md`
  - `docs/commanddeck/handoffs/COMMANDDECK-CURRENT-STATE-002-CODEX.md`

## Preview commands executed
- `pnpm.cmd run doctor:ps` (via `powershell -ExecutionPolicy Bypass -File scripts/doctor.ps1` due missing `pwsh`)
- `pnpm.cmd --filter @multica/web exec vitest run "app/(auth)/login/page.test.tsx"`
- `pnpm.cmd --filter @multica/views exec vitest run auth/login-page.test.tsx`
- `pnpm.cmd --filter @multica/desktop test`
- `pnpm.cmd --filter @multica/docs build`
- `pnpm.cmd lint`
- `pnpm.cmd build`
- `pnpm.cmd test`
- `docker compose -f compose.yml -f compose.dev.yml up -d --build`
- `docker compose -f compose.yml -f compose.dev.yml ps`
- `Invoke-WebRequest http://localhost:8080/health`
- `Invoke-WebRequest http://localhost:3000`
- `Invoke-WebRequest http://localhost:3000/login`

## Preview endpoint results
- API health: `200 OK` at `http://localhost:8080/health`
- Web root: `200 OK` at `http://localhost:3000`
- Login page: `200 OK` at `http://localhost:3000/login`
- Login content check: contains `Sign in to CommandDeck`; does not contain `Sign in to Multica`

## Build/boot failures found and fixed during recovery
- Failure: API container restart loop with DB auth failure (`password authentication failed for user "multica"`).
- Root cause: stale local compose volumes with prior DB credentials no longer matching current local `.env`.
- Recovery: `docker compose -f compose.yml -f compose.dev.yml down -v` followed by clean `up -d --build`.
- Verification: all compose services healthy/up and HTTP probes returned `200`.

## Remaining integration candidates
- Command-ledger and command-runner feature branches remain for a dedicated, conflict-aware reconciliation slice against current `origin/main`.
- High-divergence historical branches should be triaged individually before any merge.

## Next merge recommendation
Run final gate review on `integration/commanddeck-preview-recovery-005`, then merge this branch into `main` after reviewer signoff.
