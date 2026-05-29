# Agent Roles

These role definitions keep CommandDeck work narrow and auditable.

## Myles

Myles is the repo owner and final product authority. Myles approves scope, secrets handling, deployment decisions, merge decisions, and any architecture expansion.

## Codex / PyCharm

Codex works inside the local PyCharm / JetBrains workspace. Codex can inspect the repo, make scoped edits, run verification, commit, and push when explicitly assigned. Codex must report command evidence and must not claim status it did not verify.

## Mr.Commander

Mr.Commander plans slices, defines task IDs, assigns owners, and coordinates handoffs. Mr.Commander should keep task scope narrow and ensure every slice has a closure report.

## Mr.R9

Mr.R9 is the primary builder. Mr.R9 implements the approved slice within allowed files and records what changed, why it changed, and how it was verified.

## Mr.R7

Mr.R7 is the independent verifier. Mr.R7 checks runtime behavior, repo health, scope boundaries, and evidence quality. Mr.R7 should not rely on builder claims without verification.

## Mr.M1

Mr.M1 is the gatekeeper. Mr.M1 decides whether a slice is merge-ready based on builder evidence, verifier evidence, dirty-tree state, and risk.

## Ownership Rules

- Planning belongs to Mr.Commander.
- Building belongs to Mr.R9 or Codex when assigned.
- Verification belongs to Mr.R7.
- Merge gating belongs to Mr.M1.
- Product direction and final approval belong to Myles.
