FROM: Mr.M1
ROLE: Gatekeeper / Merge Authority
TASK_ID: COMMANDDECK-CLOUD-BUILDER-RECOVERY-001
VERDICT: PENDING

Repo: TheCommandDeck (https://github.com/slats55/TheCommandDeck)
Branch: agent/mr-r9/4cf1a679 (must be pushed to origin before gate review)
HEAD: aa089dc4 (baseline; new commit expected with docs changes)

## Gate Review Checklist

Mr.M1 must verify ALL of the following before issuing APPROVED:

### Builder & Build Verification
- [ ] Cloud builder `cloud-sleeper0-commanddeck-cloud` was actually created/bootstrapped (not just claimed)
- [ ] Cloud builder driver is `cloud` (not `docker` or `docker-container` local driver)
- [ ] Both API and Web images were built with `--builder cloud-sleeper0-commanddeck-cloud` flag
- [ ] Build logs or Mr.R9 report shows builds used cloud driver (not local fallback)
- [ ] No local-only builds were passed off as cloud builds

### Image & Registry Verification
- [ ] All 4 cloud tags pushed to Docker Hub (cloud-dev + cloud-\<sha\> for both API and Web)
- [ ] All 4 cloud tags are pullable (Mr.R9 verified; Mr.R7 should independently confirm)
- [ ] Images inspected and match expected architecture (linux/amd64)
- [ ] Tags follow naming convention: `sleeper0/commanddeck-<service>:cloud-<identifier>`
- [ ] `:latest` and `:dev` were NOT overwritten without explicit approval

### Docker Desktop / WSL Integration
- [ ] Docker Desktop WSL integration is working (Mr.R9 confirmed with `DOCKER_DESKTOP_WSL_RUNTIME_OK`)
- [ ] WSL2 can reach Docker daemon (not blocked by Docker Desktop settings)

### Security
- [ ] No Docker token printed in any log, comment, or output
- [ ] No Docker token committed to the repository
- [ ] No secrets (passwords, private keys, `.env` files) staged or committed
- [ ] Secret scan is CLEAN

### Runbook
- [ ] `docs/agent-brain/runbooks/DOCKER-RUNBOOK.md` has expanded "Docker Build Cloud / Cloud Builder" section
- [ ] New section includes: WSL2 verification, builder listing, inspector, create/select, API+Web cloud build+push, pull-verify, and "what to do if cloud builder does not appear"
- [ ] Runbook commands are accurate and not broken

### Handoff Documentation
- [ ] `docs/agent-brain/handoffs/COMMANDDECK-CLOUD-BUILDER-RECOVERY-001-MrR9.md` exists on origin branch
- [ ] Mr.R9 report format is complete with all required fields
- [ ] `docs/agent-brain/handoffs/COMMANDDECK-CLOUD-BUILDER-RECOVERY-001-MrR7.md` exists (verifier handoff)
- [ ] Base `compose.yml` was NOT broken or modified inappropriately

## Block Conditions

Block (do not approve) if ANY of these are true:
- Cloud builder was not actually used (local fallback only)
- Docker Desktop/WSL integration is still broken
- Docker token was printed or committed
- Images were not pull-verified
- Runbook contains fake or broken commands
- Base Compose was broken
- An agent self-approved without independent verification

## Final Report Format

```txt
FROM: Mr.M1
ROLE: Gatekeeper / Merge Authority
TASK_ID: COMMANDDECK-CLOUD-BUILDER-RECOVERY-001
VERDICT: APPROVED / BLOCKED

Repo:
Branch:
HEAD:
Builder handoff present:
Verifier handoff present:
Docker Desktop/WSL issue resolved:
Cloud builder real, not local-only:
Cloud builder name:
Cloud builder driver:
API cloud image pushed:
Web cloud image pushed:
API cloud image pull-verified:
Web cloud image pull-verified:
No secrets committed:
Runbook accurate:
Base Compose preserved:
Docker daemon exposure review:
Remaining blockers:
Final verdict:
```