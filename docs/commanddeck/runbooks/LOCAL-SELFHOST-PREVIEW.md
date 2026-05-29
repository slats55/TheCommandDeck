# Local Self-Host Preview Runbook

## Goal

Run the local source-built CommandDeck preview (not pulled official Multica images).

## Commands

```bash
pnpm install
pnpm run doctor
docker compose -f compose.yml -f compose.dev.yml up -d --build
docker compose -f compose.yml -f compose.dev.yml ps
```

## Expected Endpoints

- Web login: `http://localhost:3000/login`
- API health: `http://localhost:8080/health`

## Expected Services and Images

`docker compose -f compose.yml -f compose.dev.yml ps` should include:

- `commanddeck-commanddeck-api-1` using local-built image `commanddeck-commanddeck-api`
- `commanddeck-commanddeck-web-1` using local-built image `commanddeck-commanddeck-web`
- `commanddeck-commanddeck-db-1`
- `commanddeck-commanddeck-redis-1`

## Verify CommandDeck Branding

Run:

```powershell
$resp = Invoke-WebRequest http://localhost:3000/login -UseBasicParsing
($resp.Content -match 'Sign in to CommandDeck')
($resp.Content -match 'Sign in to Multica')
```

Expected:

- `Sign in to CommandDeck`: `True`
- `Sign in to Multica`: `False`

## Verify This Is Not Official Pulled Multica Behavior

1. Check active compose files:
   - Must use `compose.yml` + `compose.dev.yml`.
2. Check resolved config:
   - `docker compose -f compose.yml -f compose.dev.yml config` should show `build:` for `commanddeck-api` and `commanddeck-web`.
3. Do not use pull-based self-host flow when validating source-built behavior:
   - `docker-compose.selfhost.yml` defaults to GHCR image references.

## Troubleshooting

- Port conflicts:
  - Check existing containers with `docker compose -f compose.yml -f compose.dev.yml ps`.
- Stale containers:
  - Recreate with `docker compose -f compose.yml -f compose.dev.yml up -d --build --force-recreate`.
- Login flow mismatch:
  - Re-check `/login` content and confirm you are not using `docker-compose.selfhost.yml` only.
