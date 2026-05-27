#!/usr/bin/env bash
set -u

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

FAILURES=0
WARNINGS=0
COMPOSE_CONFIG_OK=false
PREVIEW_RUNNING=false
ENV_EXISTS=false

pass() { echo "PASS  $*"; }
warn() { echo "WARN  $*"; WARNINGS=$((WARNINGS + 1)); }
fail() { echo "FAIL  $*"; FAILURES=$((FAILURES + 1)); }

echo "CommandDeck repo doctor (bash)"
echo "Repo: $REPO_ROOT"
echo ""

# --- Git ---
if command -v git >/dev/null 2>&1; then
  pass "git: $(git --version | head -1)"
else
  fail "git: not found"
fi

if command -v git >/dev/null 2>&1; then
  branch="$(git branch --show-current 2>/dev/null || true)"
  if [ -n "$branch" ]; then
    pass "branch: $branch"
  else
    warn "branch: detached HEAD or unknown"
  fi

  status="$(git status --short 2>/dev/null || true)"
  if [ -z "$status" ]; then
    pass "git status: clean working tree"
  else
    warn "git status: dirty working tree"
    echo "$status" | sed 's/^/        /'
  fi

  upstream="$(git rev-parse --abbrev-ref '@{u}' 2>/dev/null || true)"
  if [ -n "$upstream" ]; then
    pass "upstream: tracking $upstream"
  else
    warn "upstream: no tracking branch configured"
  fi
fi

echo ""

# --- Node / pnpm / Docker ---
if command -v node >/dev/null 2>&1; then
  pass "node: $(node -v)"
else
  fail "node: not found"
fi

if command -v pnpm >/dev/null 2>&1; then
  pass "pnpm: $(pnpm -v)"
else
  fail "pnpm: not found"
fi

if command -v docker >/dev/null 2>&1; then
  pass "docker: $(docker --version 2>/dev/null || echo unknown)"
else
  fail "docker: not found"
fi

echo ""

# --- .env ---
if [ -f .env ]; then
  ENV_EXISTS=true
  pass ".env: present"
  if grep -qE '^JWT_SECRET=change-me-in-production' .env 2>/dev/null; then
    warn ".env: JWT_SECRET is still the default placeholder — change before shared/production use"
  fi
else
  fail ".env: missing (copy from .env.example)"
fi

echo ""

# --- Docker Compose config ---
if command -v docker >/dev/null 2>&1; then
  if docker compose -f compose.yml -f compose.dev.yml config >/dev/null 2>&1; then
    COMPOSE_CONFIG_OK=true
    pass "docker compose config: valid (compose.yml + compose.dev.yml)"
  else
    fail "docker compose config: invalid (compose.yml + compose.dev.yml)"
  fi
else
  warn "docker compose config: skipped (docker not available)"
fi

echo ""

# --- Preview probes (WARN only) ---
health_ok=false
frontend_ok=false

if command -v curl >/dev/null 2>&1; then
  if curl -sf --max-time 5 "http://localhost:8080/health" >/dev/null 2>&1; then
    health_ok=true
    pass "preview probe: http://localhost:8080/health responds"
  else
    warn "preview probe: http://localhost:8080/health not reachable"
  fi

  if curl -sf --max-time 5 "http://localhost:3000" >/dev/null 2>&1; then
    frontend_ok=true
    pass "preview probe: http://localhost:3000 responds"
  else
    warn "preview probe: http://localhost:3000 not reachable"
  fi

  if [ "$health_ok" = true ] && [ "$frontend_ok" = true ]; then
    PREVIEW_RUNNING=true
  fi
else
  warn "preview probes: skipped (curl not available)"
fi

echo ""
echo "----------------------------------------"
echo "Summary"
echo "  Hard failures: $FAILURES"
echo "  Warnings:      $WARNINGS"
echo ""

if [ "$ENV_EXISTS" = false ]; then
  echo "Next: cp .env.example .env"
elif [ "$COMPOSE_CONFIG_OK" = true ] && [ "$PREVIEW_RUNNING" = false ]; then
  echo "Next: docker compose -f compose.yml -f compose.dev.yml up -d --build"
elif [ "$PREVIEW_RUNNING" = true ]; then
  echo "Next: open http://localhost:3000/login"
else
  echo "Next: fix hard failures above, then re-run pnpm doctor"
fi

echo "----------------------------------------"

if [ "$FAILURES" -gt 0 ]; then
  exit 1
fi
exit 0
