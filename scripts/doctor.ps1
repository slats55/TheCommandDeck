# CommandDeck repo doctor for Windows PowerShell.
# Usage: pwsh -ExecutionPolicy Bypass -File scripts/doctor.ps1

$ErrorActionPreference = "Continue"

$RepoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $RepoRoot

$Failures = 0
$Warnings = 0
$ComposeConfigOk = $false
$PreviewRunning = $false
$EnvExists = $false

function Write-Pass { param([string]$Msg) Write-Host "PASS  $Msg" -ForegroundColor Green }
function Write-WarnLine {
    param([string]$Msg)
    Write-Host "WARN  $Msg" -ForegroundColor Yellow
    $script:Warnings++
}
function Write-FailLine {
    param([string]$Msg)
    Write-Host "FAIL  $Msg" -ForegroundColor Red
    $script:Failures++
}

function Test-CommandExists {
    param([string]$Name)
    $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

Write-Host "CommandDeck repo doctor (PowerShell)"
Write-Host "Repo: $RepoRoot"
Write-Host ""

# --- Git ---
if (Test-CommandExists "git") {
    Write-Pass "git: $(git --version)"
    $branch = git branch --show-current 2>$null
    if ($branch) {
        Write-Pass "branch: $branch"
    } else {
        Write-WarnLine "branch: detached HEAD or unknown"
    }

    $status = git status --short 2>$null
    if ([string]::IsNullOrWhiteSpace($status)) {
        Write-Pass "git status: clean working tree"
    } else {
        Write-WarnLine "git status: dirty working tree"
        $status -split "`n" | ForEach-Object { Write-Host "        $_" }
    }

    $upstream = git rev-parse --abbrev-ref "@{u}" 2>$null
    if ($upstream -and $LASTEXITCODE -eq 0) {
        Write-Pass "upstream: tracking $upstream"
    } else {
        Write-WarnLine "upstream: no tracking branch configured"
    }
} else {
    Write-FailLine "git: not found"
}

Write-Host ""

# --- Node / pnpm / Docker ---
if (Test-CommandExists "node") {
    Write-Pass "node: $(node -v)"
} else {
    Write-FailLine "node: not found"
}

if (Test-CommandExists "pnpm") {
    Write-Pass "pnpm: $(pnpm -v)"
} else {
    Write-FailLine "pnpm: not found"
}

if (Test-CommandExists "docker") {
    $dockerVersion = docker --version 2>$null
    Write-Pass "docker: $dockerVersion"
} else {
    Write-FailLine "docker: not found"
}

Write-Host ""

# --- .env ---
if (Test-Path ".env") {
    $EnvExists = $true
    Write-Pass ".env: present"
    $jwtLine = Select-String -Path ".env" -Pattern '^JWT_SECRET=change-me-in-production' -Quiet
    if ($jwtLine) {
        Write-WarnLine ".env: JWT_SECRET is still the default placeholder - change before shared/production use"
    }
} else {
    Write-FailLine ".env: missing (copy from .env.example)"
}

Write-Host ""

# --- Docker Compose config ---
if (Test-CommandExists "docker") {
    docker compose -f compose.yml -f compose.dev.yml config *> $null
    if ($LASTEXITCODE -eq 0) {
        $ComposeConfigOk = $true
        Write-Pass "docker compose config: valid (compose.yml + compose.dev.yml)"
    } else {
        Write-FailLine "docker compose config: invalid (compose.yml + compose.dev.yml)"
    }
} else {
    Write-WarnLine "docker compose config: skipped (docker not available)"
}

Write-Host ""

# --- Preview probes (WARN only) ---
$healthOk = $false
$frontendOk = $false

try {
    $health = Invoke-WebRequest -Uri "http://localhost:8080/health" -UseBasicParsing -TimeoutSec 5
    if ($health.StatusCode -eq 200) {
        $healthOk = $true
        Write-Pass "preview probe: http://localhost:8080/health responds"
    } else {
        Write-WarnLine "preview probe: http://localhost:8080/health returned $($health.StatusCode)"
    }
} catch {
    Write-WarnLine "preview probe: http://localhost:8080/health not reachable"
}

try {
    $frontend = Invoke-WebRequest -Uri "http://localhost:3000" -UseBasicParsing -TimeoutSec 5
    if ($frontend.StatusCode -eq 200) {
        $frontendOk = $true
        Write-Pass "preview probe: http://localhost:3000 responds"
    } else {
        Write-WarnLine "preview probe: http://localhost:3000 returned $($frontend.StatusCode)"
    }
} catch {
    Write-WarnLine "preview probe: http://localhost:3000 not reachable"
}

if ($healthOk -and $frontendOk) {
    $PreviewRunning = $true
}

Write-Host ""
Write-Host "----------------------------------------"
Write-Host "Summary"
Write-Host "  Hard failures: $Failures"
Write-Host "  Warnings:      $Warnings"
Write-Host ""

if (-not $EnvExists) {
    Write-Host "Next: Copy-Item .env.example .env"
} elseif ($ComposeConfigOk -and -not $PreviewRunning) {
    Write-Host "Next: docker compose -f compose.yml -f compose.dev.yml up -d --build"
} elseif ($PreviewRunning) {
    Write-Host "Next: open http://localhost:3000/login"
} else {
    Write-Host "Next: fix hard failures above, then re-run pnpm doctor:ps"
}

Write-Host "----------------------------------------"

if ($Failures -gt 0) {
    exit 1
}
exit 0
