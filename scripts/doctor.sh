#!/usr/bin/env bash
# CommandDeck local-dev doctor (bash).
#
# Deterministic diagnostics for the local-dev / worktree bootstrap. Reports by
# default; with --fix it performs only safe, idempotent local repairs (never
# destructive, never a DB reset, never prints secrets).
#
# Usage:
#   pnpm doctor                 # report only
#   bash scripts/doctor.sh --fix    # report + safe idempotent repairs
#   bash scripts/doctor.sh --json   # machine-readable output (for the gate runner)
set -u

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

FIX=false
JSON=false
for arg in "$@"; do
  case "$arg" in
    --fix) FIX=true ;;
    --json) JSON=true ;;
    -h|--help)
      sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) echo "doctor: unknown argument: $arg" >&2; exit 2 ;;
  esac
done

FAILURES=0
WARNINGS=0
RESULTS=()  # each entry: "STATUS<TAB>name<TAB>detail"

# In --json mode, all human output is suppressed; only the JSON document prints.
hsay() { [ "$JSON" = true ] || echo "$*"; }

record() { RESULTS+=("$1"$'\t'"$2"$'\t'"$3"); }
pass() { record PASS "$1" "$2"; hsay "PASS  $1${2:+: $2}"; }
warn() { record WARN "$1" "$2"; WARNINGS=$((WARNINGS + 1)); hsay "WARN  $1${2:+: $2}"; }
fail() { record FAIL "$1" "$2"; FAILURES=$((FAILURES + 1)); hsay "FAIL  $1${2:+: $2}"; }
note() { record NOTE "$1" "$2"; hsay "NOTE  $1${2:+: $2}"; }

json_escape() {
  # Escape backslash and double-quote; flatten tabs/newlines so the value is a
  # single safe JSON string. Messages are controlled, so this is sufficient.
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' | tr '\t\n\r' '   '
}

# Read one KEY=value from an env file without sourcing it (no code execution,
# no side effects). Returns the value with surrounding quotes stripped.
env_get() {
  local file="$1" key="$2"
  [ -f "$file" ] || return 0
  grep -E "^${key}=" "$file" 2>/dev/null | tail -1 | cut -d= -f2- | sed -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'$//"
}

# Mask the password in a postgres URL so secrets never reach stdout/logs.
mask_db_url() {
  printf '%s' "$1" | sed -E 's#(://[^:/@]+:)[^@]*@#\1****@#'
}

hsay "CommandDeck local-dev doctor (bash)"
hsay "Repo: $REPO_ROOT"
hsay ""

# --- Git ---
if command -v git >/dev/null 2>&1; then
  pass "git" "$(git --version | head -1)"
  branch="$(git branch --show-current 2>/dev/null || true)"
  if [ -n "$branch" ]; then pass "git.branch" "$branch"; else warn "git.branch" "detached HEAD or unknown"; fi
  status="$(git status --short 2>/dev/null || true)"
  if [ -z "$status" ]; then pass "git.status" "clean working tree"; else warn "git.status" "dirty working tree"; fi
else
  fail "git" "not found"
fi

# --- Worktree context ---
if [ -f .git ]; then
  IS_WORKTREE=true
  note "checkout" "linked git worktree (uses .env.worktree)"
else
  IS_WORKTREE=false
  note "checkout" "main checkout (uses .env)"
fi

hsay ""

# --- Toolchain ---
if command -v node >/dev/null 2>&1; then pass "node" "$(node -v)"; else fail "node" "not found"; fi
if command -v pnpm >/dev/null 2>&1; then pass "pnpm" "$(pnpm -v)"; else fail "pnpm" "not found"; fi
if command -v go   >/dev/null 2>&1; then
  pass "go" "$(go version 2>/dev/null | awk '{print $3}')"
else
  warn "go" "not found — required for the backend server / migrations"
fi
if command -v docker >/dev/null 2>&1; then pass "docker" "$(docker --version 2>/dev/null || echo unknown)"; else fail "docker" "not found"; fi

hsay ""

# --- Active env file (mirrors the Makefile: .env if present, else .env.worktree) ---
ENV_FILE=""
if [ -f .env ]; then
  ENV_FILE=".env"
elif [ -f .env.worktree ]; then
  ENV_FILE=".env.worktree"
fi

if [ -n "$ENV_FILE" ]; then
  pass "env.file" "$ENV_FILE present"
  if grep -qE '^JWT_SECRET=change-me-in-production' "$ENV_FILE" 2>/dev/null; then
    warn "env.jwt" "JWT_SECRET is still the default placeholder — change before shared/production use"
  fi
else
  if [ "$IS_WORKTREE" = true ]; then
    fail "env.file" "no .env or .env.worktree — run 'make worktree-env' (or doctor --fix)"
  else
    fail "env.file" "no .env — copy from .env.example"
  fi
fi

# Pull the values doctor needs (no secrets echoed).
POSTGRES_USER="$(env_get "$ENV_FILE" POSTGRES_USER)"; POSTGRES_USER="${POSTGRES_USER:-multica}"
POSTGRES_DB="$(env_get "$ENV_FILE" POSTGRES_DB)"; POSTGRES_DB="${POSTGRES_DB:-multica}"
DATABASE_URL="$(env_get "$ENV_FILE" DATABASE_URL)"
PORT="$(env_get "$ENV_FILE" PORT)"; PORT="${PORT:-8080}"
FRONTEND_PORT="$(env_get "$ENV_FILE" FRONTEND_PORT)"; FRONTEND_PORT="${FRONTEND_PORT:-3000}"

if [ -n "$DATABASE_URL" ]; then
  pass "db.url" "$(mask_db_url "$DATABASE_URL")"
else
  note "db.url" "DATABASE_URL not set in $ENV_FILE (will default to local postgres)"
fi

hsay ""

# --- Docker Compose topology (the root cause this doctor was built to catch) ---
HAS_COMPOSE_YML=false; [ -f compose.yml ] && HAS_COMPOSE_YML=true
HAS_DOCKER_COMPOSE_YML=false; [ -f docker-compose.yml ] && HAS_DOCKER_COMPOSE_YML=true

if [ "$HAS_COMPOSE_YML" = true ] && [ "$HAS_DOCKER_COMPOSE_YML" = true ]; then
  note "compose.ambiguity" "compose.yml AND docker-compose.yml both exist — a bare 'docker compose' auto-selects compose.yml (the production stack, no 'postgres' service). Local-dev DB commands MUST use 'docker compose -f docker-compose.yml'."
fi

if command -v docker >/dev/null 2>&1 && [ "$HAS_DOCKER_COMPOSE_YML" = true ]; then
  localdb_services="$(docker compose -f docker-compose.yml config --services 2>/dev/null || true)"
  if printf '%s\n' "$localdb_services" | grep -qx "postgres"; then
    pass "compose.localdb" "docker-compose.yml defines the 'postgres' service"
  else
    fail "compose.localdb" "docker-compose.yml is missing the 'postgres' service"
  fi
elif [ "$HAS_DOCKER_COMPOSE_YML" = false ]; then
  fail "compose.localdb" "docker-compose.yml not found at repo root"
else
  warn "compose.localdb" "skipped (docker not available)"
fi

hsay ""

# --- Local-dev DB container + readiness (pinned to docker-compose.yml) ---
if command -v docker >/dev/null 2>&1 && [ "$HAS_DOCKER_COMPOSE_YML" = true ]; then
  cid="$(docker compose -f docker-compose.yml ps -q postgres 2>/dev/null || true)"
  if [ -n "$cid" ]; then
    running="$(docker inspect -f '{{.State.Running}}' "$cid" 2>/dev/null || echo false)"
    if [ "$running" = "true" ]; then
      pass "db.container" "postgres container running"
      if docker compose -f docker-compose.yml exec -T postgres pg_isready -U "$POSTGRES_USER" -d postgres >/dev/null 2>&1; then
        pass "db.ready" "PostgreSQL accepting connections"
        if [ -n "$POSTGRES_DB" ]; then
          db_exists="$(docker compose -f docker-compose.yml exec -T postgres psql -U "$POSTGRES_USER" -d postgres -Atqc "SELECT 1 FROM pg_database WHERE datname = '$POSTGRES_DB'" 2>/dev/null || true)"
          if [ "$db_exists" = "1" ]; then
            pass "db.database" "database '$POSTGRES_DB' exists"
          else
            warn "db.database" "database '$POSTGRES_DB' does not exist yet — run 'make setup' (or doctor --fix)"
          fi
        fi
      else
        warn "db.ready" "postgres container is up but not accepting connections yet"
      fi
    else
      warn "db.container" "postgres container exists but is stopped — run 'make db-up' (or doctor --fix)"
    fi
  else
    warn "db.container" "no postgres container — run 'make db-up' (or doctor --fix)"
  fi
else
  warn "db.container" "skipped (docker not available)"
fi

hsay ""

# --- Production/self-host compose validity (informational; not the local-dev path) ---
if command -v docker >/dev/null 2>&1; then
  if docker compose -f compose.yml -f compose.dev.yml config >/dev/null 2>&1; then
    pass "compose.stack" "compose.yml + compose.dev.yml config valid"
  else
    warn "compose.stack" "compose.yml + compose.dev.yml config invalid (only needed for the container preview path)"
  fi
fi

hsay ""

# --- Preview liveness probes (env-aware ports; WARN only) ---
if command -v curl >/dev/null 2>&1; then
  if curl -sf --max-time 5 "http://localhost:${PORT}/health" >/dev/null 2>&1; then
    pass "preview.server" "http://localhost:${PORT}/health responds"
  else
    warn "preview.server" "http://localhost:${PORT}/health not reachable (server may not be started)"
  fi
  if curl -sf --max-time 5 "http://localhost:${FRONTEND_PORT}" >/dev/null 2>&1; then
    pass "preview.web" "http://localhost:${FRONTEND_PORT} responds"
  else
    warn "preview.web" "http://localhost:${FRONTEND_PORT} not reachable (web may not be started)"
  fi
else
  warn "preview" "skipped (curl not available)"
fi

note "optional" "desktop app and the agent daemon are optional — not required for a full-stack web preview"

# --- Safe, idempotent repairs (only with --fix) ---
if [ "$FIX" = true ]; then
  hsay ""
  hsay "--- doctor --fix: safe idempotent repairs ---"
  if [ -z "$ENV_FILE" ] && [ "$IS_WORKTREE" = true ]; then
    hsay "==> generating .env.worktree"
    bash scripts/init-worktree-env.sh .env.worktree && ENV_FILE=".env.worktree"
  fi
  if [ -n "$ENV_FILE" ] && command -v docker >/dev/null 2>&1; then
    hsay "==> ensuring local PostgreSQL via scripts/ensure-postgres.sh"
    bash scripts/ensure-postgres.sh "$ENV_FILE" || true
  fi
  if command -v go >/dev/null 2>&1; then
    hsay "==> migrations are not auto-run; apply them with:"
    hsay "    cd server && go run ./cmd/migrate up"
  fi
  hsay "Re-run 'pnpm doctor' to confirm."
fi

# --- Output ---
if [ "$JSON" = true ]; then
  printf '{\n'
  printf '  "repoRoot": "%s",\n' "$(json_escape "$REPO_ROOT")"
  printf '  "envFile": "%s",\n' "$(json_escape "$ENV_FILE")"
  printf '  "failures": %d,\n' "$FAILURES"
  printf '  "warnings": %d,\n' "$WARNINGS"
  printf '  "checks": [\n'
  first=true
  for r in "${RESULTS[@]}"; do
    st="${r%%$'\t'*}"; rest="${r#*$'\t'}"; nm="${rest%%$'\t'*}"; dt="${rest#*$'\t'}"
    if [ "$first" = true ]; then first=false; else printf ',\n'; fi
    printf '    {"name": "%s", "status": "%s", "detail": "%s"}' "$(json_escape "$nm")" "$st" "$(json_escape "$dt")"
  done
  printf '\n  ]\n}\n'
else
  echo ""
  echo "----------------------------------------"
  echo "Summary"
  echo "  Hard failures: $FAILURES"
  echo "  Warnings:      $WARNINGS"
  echo ""
  if [ -z "$ENV_FILE" ]; then
    if [ "$IS_WORKTREE" = true ]; then
      echo "Next: make worktree-env   (or: bash scripts/doctor.sh --fix)"
    else
      echo "Next: cp .env.example .env"
    fi
  elif [ "$FAILURES" -gt 0 ]; then
    echo "Next: fix the hard failures above, then re-run 'pnpm doctor'"
  else
    echo "Next: make start-worktree   (then open http://localhost:${FRONTEND_PORT}/login)"
  fi
  echo "----------------------------------------"
fi

[ "$FAILURES" -gt 0 ] && exit 1
exit 0
