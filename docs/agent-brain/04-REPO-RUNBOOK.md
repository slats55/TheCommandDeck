# Repo Runbook

This runbook defines the standard local hygiene flow for CommandDeck agents.

## Start Of Slice

```bash
cd C:/Users/mtval/TheCommandDeck
git rev-parse --show-toplevel
git branch --show-current
git status --short
git fetch origin --prune
```

If the working tree is dirty, inspect before proceeding:

```bash
git diff --stat
git diff --cached --stat
git diff
git diff --cached
```

## Update Main

Only update `main` from a clean working tree:

```bash
git checkout main
git pull --ff-only origin main
git rev-list --left-right --count HEAD...origin/main
git status --short
```

Expected sync count:

```text
0    0
```

## Create A Slice Branch

```bash
git checkout -b <branch-name>
```

Use branch names that include the task scope and task number.

## Standard Health Check

```bash
pnpm run doctor
```

Expected closeout condition:

- hard failures: `0`
- warnings: explained in the closure report

The `.env` JWT placeholder warning can appear locally. It is not a hard failure when the doctor reports zero hard failures, but it must be fixed before shared or production deployment.

## Closeout

```bash
git status --short
git diff --stat
pnpm run doctor
git add <allowed-files>
git commit -m "<message>"
git push -u origin <branch-name>
git fetch origin --prune
git rev-list --left-right --count HEAD...origin/<branch-name>
git status --short
```

The branch is synced only when the final count is:

```text
0    0
```

## GitHub Auth Notes

- GitHub HTTPS auth is expected to work through Git Credential Manager.
- Do not print, paste, or store GitHub tokens in docs, shell history, logs, or chat.
- If auth fails, authenticate manually through PyCharm GitHub settings or Git Credential Manager.
