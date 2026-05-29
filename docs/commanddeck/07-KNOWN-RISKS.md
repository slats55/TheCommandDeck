# CommandDeck Known Risks

## Build and Test Risks

- JS/TS monorepo gates currently pass (`pnpm lint`, `pnpm build`, `pnpm test`), but Go gate verification is blocked in this environment because `go` is not installed.
- Desktop tests still emit existing warnings around local storage flags in Node; tests pass but warning cleanup is pending.

## Auth and Self-Host Risks

- Local login branding is CommandDeck, but auth flow still depends on existing backend auth behavior and current env configuration.
- Self-host docs and config include legacy `MULTICA_*` environment variable names that may cause operator confusion.

## Cloud Reference Risks

- Some repository docs and self-host instructions still reference official Multica cloud/images (`multica setup self-host`, GHCR image paths).
- If operators use `docker-compose.selfhost.yml` pull-based flow without overrides, they may run official images instead of source-built CommandDeck services.

## Command Execution Risks (Future Work)

- Introducing command execution before runtime identity/workspace boundaries/audit logs are in place would create unacceptable security risk.
- Raw arbitrary shell execution is out of scope for MVP and must stay disabled until explicit security controls are implemented.
