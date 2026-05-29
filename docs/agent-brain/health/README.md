# Health Reports

Use this folder for repo health snapshots and verification reports.

Standard local health command:

```bash
pnpm run doctor
```

Health reports should capture:

- repo root
- branch
- HEAD
- dirty tree status
- GitHub auth readiness when relevant
- doctor hard failures
- doctor warnings
- runtime probes when relevant

Do not mark a report as passing unless the command evidence supports it.
