# CommandDeck local-dev doctor (Windows PowerShell).
#
# Deterministic diagnostics for the local-dev / worktree bootstrap. Reports by
# default; with -Fix it performs only safe, idempotent local repairs (never
# destructive, never a DB reset, never prints secrets).
#
# Usage:
#   pnpm doctor:ps
#   pwsh -ExecutionPolicy Bypass -File scripts/doctor.ps1 -Fix
#   pwsh -ExecutionPolicy Bypass -File scripts/doctor.ps1 -Json
param(
    [switch]$Fix,
    [switch]$Json
)

$ErrorActionPreference = "Continue"

$RepoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $RepoRoot

$Failures = 0
$Warnings = 0
$Results = New-Object System.Collections.ArrayList

function Say { param([string]$Msg, [string]$Color = "Gray") if (-not $Json) { Write-Host $Msg -ForegroundColor $Color } }

function Add-Result {
    param([string]$Status, [string]$Name, [string]$Detail)
    [void]$Results.Add([pscustomobject]@{ name = $Name; status = $Status; detail = $Detail })
}
function Write-Pass { param([string]$Name, [string]$Detail) Add-Result "PASS" $Name $Detail; if (-not $Json) { Write-Host ("PASS  {0}{1}" -f $Name, $(if ($Detail) { ": $Detail" })) -ForegroundColor Green } }
function Write-WarnLine { param([string]$Name, [string]$Detail) Add-Result "WARN" $Name $Detail; $script:Warnings++; if (-not $Json) { Write-Host ("WARN  {0}{1}" -f $Name, $(if ($Detail) { ": $Detail" })) -ForegroundColor Yellow } }
function Write-FailLine { param([string]$Name, [string]$Detail) Add-Result "FAIL" $Name $Detail; $script:Failures++; if (-not $Json) { Write-Host ("FAIL  {0}{1}" -f $Name, $(if ($Detail) { ": $Detail" })) -ForegroundColor Red } }
function Write-NoteLine { param([string]$Name, [string]$Detail) Add-Result "NOTE" $Name $Detail; if (-not $Json) { Write-Host ("NOTE  {0}{1}" -f $Name, $(if ($Detail) { ": $Detail" })) -ForegroundColor Cyan } }

function Test-CommandExists { param([string]$Name) $null -ne (Get-Command $Name -ErrorAction SilentlyContinue) }

function Get-EnvValue {
    param([string]$File, [string]$Key)
    if (-not $File -or -not (Test-Path $File)) { return $null }
    $line = Select-String -Path $File -Pattern "^$Key=" -ErrorAction SilentlyContinue | Select-Object -Last 1
    if (-not $line) { return $null }
    $v = $line.Line.Substring($line.Line.IndexOf('=') + 1)
    return $v.Trim('"').Trim("'")
}

function Format-MaskedDbUrl { param([string]$Url) if (-not $Url) { return $Url }; return ($Url -replace '(://[^:/@]+:)[^@]*@', '${1}****@') }

Say "CommandDeck local-dev doctor (PowerShell)"
Say "Repo: $RepoRoot"
Say ""

# --- Git ---
if (Test-CommandExists "git") {
    Write-Pass "git" "$(git --version)"
    $branch = git branch --show-current 2>$null
    if ($branch) { Write-Pass "git.branch" "$branch" } else { Write-WarnLine "git.branch" "detached HEAD or unknown" }
    $status = git status --short 2>$null
    if ([string]::IsNullOrWhiteSpace($status)) { Write-Pass "git.status" "clean working tree" } else { Write-WarnLine "git.status" "dirty working tree" }
} else {
    Write-FailLine "git" "not found"
}

# --- Worktree context ---
$IsWorktree = Test-Path ".git" -PathType Leaf
if ($IsWorktree) { Write-NoteLine "checkout" "linked git worktree (uses .env.worktree)" }
else { Write-NoteLine "checkout" "main checkout (uses .env)" }

Say ""

# --- Toolchain ---
if (Test-CommandExists "node") { Write-Pass "node" "$(node -v)" } else { Write-FailLine "node" "not found" }
if (Test-CommandExists "pnpm") { Write-Pass "pnpm" "$(pnpm -v)" } else { Write-FailLine "pnpm" "not found" }
if (Test-CommandExists "go") {
    $goVer = (go version 2>$null) -split ' '
    Write-Pass "go" "$($goVer[2])"
} else {
    Write-WarnLine "go" "not found - required for the backend server / migrations"
}
if (Test-CommandExists "docker") { Write-Pass "docker" "$(docker --version 2>$null)" } else { Write-FailLine "docker" "not found" }

Say ""

# --- Active env file (mirrors the Makefile: .env if present, else .env.worktree) ---
$EnvFile = ""
if (Test-Path ".env") { $EnvFile = ".env" }
elseif (Test-Path ".env.worktree") { $EnvFile = ".env.worktree" }

if ($EnvFile) {
    Write-Pass "env.file" "$EnvFile present"
    if (Select-String -Path $EnvFile -Pattern '^JWT_SECRET=change-me-in-production' -Quiet -ErrorAction SilentlyContinue) {
        Write-WarnLine "env.jwt" "JWT_SECRET is still the default placeholder - change before shared/production use"
    }
} else {
    if ($IsWorktree) { Write-FailLine "env.file" "no .env or .env.worktree - run 'make worktree-env' (or doctor -Fix)" }
    else { Write-FailLine "env.file" "no .env - copy from .env.example" }
}

$PostgresUser = Get-EnvValue $EnvFile "POSTGRES_USER"; if (-not $PostgresUser) { $PostgresUser = "multica" }
$PostgresDb = Get-EnvValue $EnvFile "POSTGRES_DB"; if (-not $PostgresDb) { $PostgresDb = "multica" }
$DatabaseUrl = Get-EnvValue $EnvFile "DATABASE_URL"
$Port = Get-EnvValue $EnvFile "PORT"; if (-not $Port) { $Port = "8080" }
$FrontendPort = Get-EnvValue $EnvFile "FRONTEND_PORT"; if (-not $FrontendPort) { $FrontendPort = "3000" }

if ($DatabaseUrl) { Write-Pass "db.url" "$(Format-MaskedDbUrl $DatabaseUrl)" }
else { Write-NoteLine "db.url" "DATABASE_URL not set in $EnvFile (will default to local postgres)" }

Say ""

# --- Docker Compose topology (the root cause this doctor was built to catch) ---
$HasComposeYml = Test-Path "compose.yml"
$HasDockerComposeYml = Test-Path "docker-compose.yml"

if ($HasComposeYml -and $HasDockerComposeYml) {
    Write-NoteLine "compose.ambiguity" "compose.yml AND docker-compose.yml both exist - a bare 'docker compose' auto-selects compose.yml (the production stack, no 'postgres' service). Local-dev DB commands MUST use 'docker compose -f docker-compose.yml'."
}

if ((Test-CommandExists "docker") -and $HasDockerComposeYml) {
    $localdbServices = docker compose -f docker-compose.yml config --services 2>$null
    if ($localdbServices -contains "postgres") {
        Write-Pass "compose.localdb" "docker-compose.yml defines the 'postgres' service"
    } else {
        Write-FailLine "compose.localdb" "docker-compose.yml is missing the 'postgres' service"
    }
} elseif (-not $HasDockerComposeYml) {
    Write-FailLine "compose.localdb" "docker-compose.yml not found at repo root"
} else {
    Write-WarnLine "compose.localdb" "skipped (docker not available)"
}

Say ""

# --- Local-dev DB container + readiness (pinned to docker-compose.yml) ---
if ((Test-CommandExists "docker") -and $HasDockerComposeYml) {
    $cid = docker compose -f docker-compose.yml ps -q postgres 2>$null
    if ($cid) {
        $running = docker inspect -f '{{.State.Running}}' $cid 2>$null
        if ($running -eq "true") {
            Write-Pass "db.container" "postgres container running"
            docker compose -f docker-compose.yml exec -T postgres pg_isready -U $PostgresUser -d postgres *> $null
            if ($LASTEXITCODE -eq 0) {
                Write-Pass "db.ready" "PostgreSQL accepting connections"
                $dbExists = docker compose -f docker-compose.yml exec -T postgres psql -U $PostgresUser -d postgres -Atqc "SELECT 1 FROM pg_database WHERE datname = '$PostgresDb'" 2>$null
                if ($dbExists -match "1") { Write-Pass "db.database" "database '$PostgresDb' exists" }
                else { Write-WarnLine "db.database" "database '$PostgresDb' does not exist yet - run 'make setup' (or doctor -Fix)" }
            } else {
                Write-WarnLine "db.ready" "postgres container is up but not accepting connections yet"
            }
        } else {
            Write-WarnLine "db.container" "postgres container exists but is stopped - run 'make db-up' (or doctor -Fix)"
        }
    } else {
        Write-WarnLine "db.container" "no postgres container - run 'make db-up' (or doctor -Fix)"
    }
} else {
    Write-WarnLine "db.container" "skipped (docker not available)"
}

Say ""

# --- Production/self-host compose validity (informational; not the local-dev path) ---
if (Test-CommandExists "docker") {
    docker compose -f compose.yml -f compose.dev.yml config *> $null
    if ($LASTEXITCODE -eq 0) { Write-Pass "compose.stack" "compose.yml + compose.dev.yml config valid" }
    else { Write-WarnLine "compose.stack" "compose.yml + compose.dev.yml config invalid (only needed for the container preview path)" }
}

Say ""

# --- Preview liveness probes (env-aware ports; WARN only) ---
try {
    $r = Invoke-WebRequest -Uri "http://localhost:$Port/health" -UseBasicParsing -TimeoutSec 5
    if ($r.StatusCode -eq 200) { Write-Pass "preview.server" "http://localhost:$Port/health responds" }
    else { Write-WarnLine "preview.server" "http://localhost:$Port/health returned $($r.StatusCode)" }
} catch { Write-WarnLine "preview.server" "http://localhost:$Port/health not reachable (server may not be started)" }

try {
    $r = Invoke-WebRequest -Uri "http://localhost:$FrontendPort" -UseBasicParsing -TimeoutSec 5
    if ($r.StatusCode -eq 200) { Write-Pass "preview.web" "http://localhost:$FrontendPort responds" }
    else { Write-WarnLine "preview.web" "http://localhost:$FrontendPort returned $($r.StatusCode)" }
} catch { Write-WarnLine "preview.web" "http://localhost:$FrontendPort not reachable (web may not be started)" }

Write-NoteLine "optional" "desktop app and the agent daemon are optional - not required for a full-stack web preview"

# --- Safe, idempotent repairs (only with -Fix) ---
if ($Fix) {
    Say ""
    Say "--- doctor -Fix: safe idempotent repairs ---"
    if (-not $EnvFile -and $IsWorktree) {
        Say "==> generating .env.worktree"
        bash scripts/init-worktree-env.sh .env.worktree
        if (Test-Path ".env.worktree") { $EnvFile = ".env.worktree" }
    }
    if ($EnvFile -and (Test-CommandExists "docker")) {
        Say "==> ensuring local PostgreSQL via scripts/ensure-postgres.sh"
        bash scripts/ensure-postgres.sh $EnvFile
    }
    if (Test-CommandExists "go") {
        Say "==> migrations are not auto-run; apply them with:"
        Say "    cd server && go run ./cmd/migrate up"
    }
    Say "Re-run 'pnpm doctor:ps' to confirm."
}

# --- Output ---
if ($Json) {
    $doc = [pscustomobject]@{
        repoRoot = $RepoRoot
        envFile  = $EnvFile
        failures = $Failures
        warnings = $Warnings
        checks   = $Results
    }
    $doc | ConvertTo-Json -Depth 4
} else {
    Write-Host ""
    Write-Host "----------------------------------------"
    Write-Host "Summary"
    Write-Host "  Hard failures: $Failures"
    Write-Host "  Warnings:      $Warnings"
    Write-Host ""
    if (-not $EnvFile) {
        if ($IsWorktree) { Write-Host "Next: make worktree-env   (or: pwsh -File scripts/doctor.ps1 -Fix)" }
        else { Write-Host "Next: Copy-Item .env.example .env" }
    } elseif ($Failures -gt 0) {
        Write-Host "Next: fix the hard failures above, then re-run 'pnpm doctor:ps'"
    } else {
        Write-Host "Next: make start-worktree   (then open http://localhost:$FrontendPort/login)"
    }
    Write-Host "----------------------------------------"
}

if ($Failures -gt 0) { exit 1 }
exit 0
