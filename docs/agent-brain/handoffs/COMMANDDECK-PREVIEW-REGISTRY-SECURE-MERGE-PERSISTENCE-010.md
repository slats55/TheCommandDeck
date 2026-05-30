# COMMANDDECK-PREVIEW-REGISTRY-SECURE-MERGE-PERSISTENCE-010

## Scope

Secured the Preview Registry MVP before merge. The feature remains limited to the CommandDeck preview registry API/client/types/UI surface and related tests.

## Security Issues Addressed

- Raw backend probe errors are no longer returned to the browser.
- Preview health probe redirects are not followed.
- Configured preview targets are validated before probing.
- Unsupported schemes, malformed URLs, userinfo URLs, private non-local IP targets, and untrusted local/internal hostnames are rejected or safely omitted.
- The API remains server-derived only; no request body or query parameter can supply a health-check URL.
- The UI renders safe health messages and does not render raw `health_error` values.

## Files Changed

- `server/internal/handler/previewregistry.go`
- `server/internal/handler/previewregistry_test.go`
- `packages/core/api/client.ts`
- `packages/core/api/client.test.ts`
- `packages/core/api/schemas.ts`
- `packages/core/types/commanddeck.ts`
- `apps/web/app/[workspaceSlug]/(dashboard)/commanddeck/page.tsx`
- `apps/web/app/[workspaceSlug]/(dashboard)/commanddeck/page.test.tsx`

## Test Coverage Added

- Backend target validation for trusted local preview targets and unsafe target rejection.
- Backend health probe success, unavailable transport failure sanitization, redirect no-follow behavior, and bounded timeout.
- Core API client schema parsing and fixed server-derived registry endpoint behavior.
- Web CommandDeck page tests for healthy, loading, empty, and unavailable registry states.
- UI test proves raw internal `health_error` text from the API shape is not rendered.

## Verification Evidence

- `powershell -ExecutionPolicy Bypass -File scripts/doctor.ps1`: passed; dirty tree warning only.
- `pnpm lint`: passed with existing unrelated warnings.
- `pnpm build`: passed.
- `pnpm test`: passed.
- Docker Go test: `docker run --rm -v C:\Users\mtval\TheCommandDeck:/src -w /src/server golang:1.26-alpine sh -lc "apk add --no-cache git >/dev/null && /usr/local/go/bin/go test ./..."`: passed.
- Docker Go build: `docker run --rm -v C:\Users\mtval\TheCommandDeck:/src -w /src/server golang:1.26-alpine sh -lc "apk add --no-cache git >/dev/null && /usr/local/go/bin/go build ./..."`: passed.
- Docker compose rebuild/start: passed.
- API health: `http://localhost:8080/health` returned 200.
- Web preview: `http://localhost:3000` returned 200.
- Login preview: `http://localhost:3000/login` returned 200 and showed CommandDeck branding.

## Live Product Evidence

- Authenticated registry API proof:
  - Workspace: `ui-b6b09e8ba922`
  - Runtime: `7ac9cc06-811d-404f-99a7-32513e0f5a8d`
  - Preview URL: `http://localhost:3000`
  - Port: `3000`
  - Health: `healthy`
  - `health_error` exposed: `false`
- Authenticated browser proof:
  - `http://localhost:3000/ui-b6b09e8ba922/commanddeck`
  - Preview Registry panel rendered.
  - URL, port, runtime, and healthy state rendered from real API data.
  - No raw internal network error text rendered.
- Command-runner regression proof:
  - Workspace: `cmd-7421df19d14c`
  - Runtime: `5cfa0fbc-8b10-4a99-931b-5ba5b876cc3a`
  - Command: `git status`
  - Run: `cb772111-c800-4a3b-ba03-ac53aab551e5`
  - Status: `completed`
  - Exit code: `0`
  - Ledger entries: `1`

## Merge Recommendation

The secured Preview Registry MVP is ready to merge into `main` after the feature branch commit is pushed and post-merge gates repeat successfully.

## Next Slice

Create `feature/commanddeck-preview-health-persistence-011` from updated `main` and persist trusted self-hosted preview health records using the existing Go/PostgreSQL/SQLC architecture.
