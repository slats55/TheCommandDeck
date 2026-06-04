# Local Dogfood Access Runbook

## Goal

Open the locally self-hosted CommandDeck instance, sign in through the safe
local-development path, reach the Command Deck dashboard, and verify real
CommandDeck functionality from the browser.

This is the access-first companion to [LOCAL-SELFHOST-PREVIEW.md](LOCAL-SELFHOST-PREVIEW.md),
which covers building and starting the source-built stack.

> **Security:** every step below is for **non-production** local development
> only. Development sign-in MUST NOT be enabled on a production deployment — see
> "Production safety" at the end.

## 1. Start the stack

```bash
docker compose -f compose.yml -f compose.dev.yml up -d --build
docker compose -f compose.yml -f compose.dev.yml ps
```

Expected services (all `Up`):

- `commanddeck-commanddeck-api-1` (`:8080`)
- `commanddeck-commanddeck-web-1` (`:3000`)
- `commanddeck-commanddeck-db-1` (`:5432`, healthy)
- `commanddeck-commanddeck-redis-1` (`:6379`, healthy)

API liveness:

```bash
curl http://localhost:8080/health    # -> 200
```

## 2. Required local-only environment

These are set in your local `.env` (never commit `.env`). They apply to the
`commanddeck-api` service:

| Variable | Local dogfood value | Purpose |
| --- | --- | --- |
| `APP_ENV` | `development` | Must NOT be `production` for dev sign-in |
| `MULTICA_DEV_VERIFICATION_CODE` | a six-digit code, e.g. `888888` | Deterministic local login code |
| `ALLOW_SIGNUP` | `true` | Lets a first local operator account be created |
| `JWT_SECRET` | any non-default local value | Signs local sessions |

Do not print or commit `JWT_SECRET`. The verification code is a deliberate
local-only convenience; it is never returned by the public API (see step 6).

When these are set, the public config endpoint reports a boolean hint (and only
a boolean — never the code):

```bash
curl -s http://localhost:8080/api/config
# { ... "dev_auth_enabled": true ... }
```

If you change any of these values, restart the API so it re-reads them:

```bash
docker compose -f compose.yml -f compose.dev.yml up -d commanddeck-api
```

## 3. Sign in (local development path)

1. Open `http://localhost:3000/login`.
2. The screen shows the **CommandDeck** wordmark and, because
   `dev_auth_enabled` is true, a **"Local development sign-in"** notice.
3. Enter an operator email and click **Continue**. For a fresh local instance
   use a dedicated local identity, e.g. `operator@commanddeck.local`.
   - `SendCode` creates a verification record. With no email provider
     configured locally, the real one-time code is printed to the API logs
     instead of emailed:
     ```bash
     docker logs --tail 20 commanddeck-commanddeck-api-1 | grep "\[DEV\]"
     # [DEV] Verification code for operator@commanddeck.local: 1234 56
     ```
4. On the code screen, enter the configured development code (`888888`).
   - The development code is accepted **only** in non-production and **only**
     after a verification record exists (i.e. after step 3 submits the email).

You are now authenticated.

## 4. Reach the Command Deck dashboard

- After login you land on your workspace (or onboarding if the account has
  none yet).
- In the left sidebar, under **Configure**, click **Command Deck** (terminal
  icon).
- URL: `http://localhost:3000/<workspace-slug>/commanddeck`

### Find your workspace slug

```bash
docker exec commanddeck-commanddeck-db-1 \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
  "SELECT slug, name FROM workspace ORDER BY created_at DESC;"
```

(`POSTGRES_USER`/`POSTGRES_DB` default to the values in your `.env`.)

## 5. Verify real functionality on the page

The Command Deck page renders four real, workspace-scoped panels:

- **Preview Registry**
- **Runtime Health**
- **Run a Command**
- **Workflow Execution Records**

### Verify runtime online state

- **Runtime Health** lists this workspace's runtimes and their true status.
- A runtime shows **online/healthy** only while its daemon is sending
  heartbeats. If the host daemon is stopped, the panel truthfully shows the
  runtime as **offline** — it is never faked.
- To bring a runtime online, start the local daemon for this workspace, then
  refresh Runtime Health.

### Run one allowlisted command

1. In **Run a Command**, pick an online runtime.
2. Choose a safe allowlisted template — `Git Status` (`git status`) is the
   safest.
3. Run it. The panel shows the real run status and bounded output. Cancellation
   is run-scoped.
- If no runtime is online, command execution is unavailable by design (there is
  nothing to execute against). This is correct, truthful behavior, not a bug.

### Register / refresh a preview

- Use the **Preview Registry** panel's existing controls to register or refresh
  the current self-hosted preview.
- Interpret status truthfully: **empty** (none registered), **offline/stale**
  (last check failed or aged out), or **healthy** (reachable). The UI never
  fabricates a healthy state.

## 6. Security checks (what "safe" means here)

- The public `/api/config` endpoint exposes only the boolean
  `dev_auth_enabled`. It never returns `MULTICA_DEV_VERIFICATION_CODE`.
- `dev_auth_enabled` is computed with the same gate the backend applies before
  accepting the dev code: non-production **and** a valid six-digit code
  configured. In production it is always `false`.

## Production safety

- Never set `MULTICA_DEV_VERIFICATION_CODE` on a production deployment, and
  always run production with `APP_ENV=production`. Under those settings the
  development sign-in path is disabled and the login screen shows no dev notice.
- The development code grants login without an emailed code. Treat any instance
  where it is enabled as a development instance only.

## Troubleshooting

- **No dev notice on the login screen** — confirm `APP_ENV=development` and a
  valid six-digit `MULTICA_DEV_VERIFICATION_CODE`, then restart `commanddeck-api`
  and re-check `curl http://localhost:8080/api/config`.
- **`SendCode` errors** — check `docker logs commanddeck-commanddeck-api-1`. With
  no email provider configured the `[DEV]` log line should still appear and the
  request should return 200.
- **Login screen still shows old branding** — you are on a stale web image;
  rebuild with `--build` (see step 1).
- **Runtime shows offline** — the daemon is not heartbeating. Start the local
  daemon for the workspace; this is reported truthfully, not mocked.
