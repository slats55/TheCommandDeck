# CommandDeck Agent Brain

This folder is the durable operating memory for CommandDeck agents. It records how agents should reason about the repo, how tasks are handed off, what evidence is required, and which operational rules are non-negotiable.

The agent brain is documentation-only unless a task explicitly approves code or configuration changes. It should help future agents start from evidence instead of assumptions.

## Read Order

1. `00-START-HERE.md` - purpose and navigation.
2. `01-CURRENT-STATE.md` - current repo baseline and known architecture facts.
3. `02-AGENT-ROLES.md` - ownership boundaries for humans and agents.
4. `03-WORKFLOW-RULES.md` - task hygiene, handoffs, and closure rules.
5. `04-REPO-RUNBOOK.md` - local Git and repo health workflow.
6. `05-KNOWN-ISSUES.md` - active warnings, blockers, and watch items.

## Ground Rules

- Confirm repo root before acting.
- Confirm current branch before acting.
- Check dirty tree before branch work.
- Fetch before starting a new branch.
- Use `git pull --ff-only` when updating `main`.
- Never claim push, merge, runtime, or verification status without command evidence.
- Do not produce fake completion reports.
- Do not start a new slice until the previous slice has a closure report.

## Scope

This folder is for CommandDeck operating memory:

- decisions
- handoffs
- health reports
- runbooks
- role boundaries
- verification evidence
- known operational issues

Product behavior, runtime code, UI code, migrations, auth, and CI should not be changed from this folder's tasks unless a task explicitly authorizes that scope.
