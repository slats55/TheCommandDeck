# COMMANDDECK-PREVIEW-PERSISTENCE-GATE-MERGE-REFRESH-012

## Starting points

- `origin/main`: `00e911f63d4e18f1deb09bc8cee5b5b9b361e896`
- `origin/feature/commanddeck-preview-health-persistence-011`: `d40acbd051e9eb0e1a929ca047534cdf87b90cdc`
- Relationship: feature was one commit ahead of main and not merged.

## Findings

1. `GET /api/commandrunner/previews` performed `UpsertPreviewRegistryRecord` on every poll.
2. Self-hosted preview runtime provenance was inferred from arbitrary online runtime selection.
3. Trusted target policy still allowed broad HTTPS hostnames.

## Resolutions

- Made GET preview listing read-only (no durable writes).
- Added explicit trusted mutation endpoint:
  - `POST /api/commandrunner/previews/self-hosted/sync`
  - Accepts no client URL input.
  - Uses only server-derived trusted preview target and bounded no-redirect probe.
- Enforced self-hosted provenance correctness:
  - runtime/machine/command-run linkage is not auto-populated without proof.
- Tightened target policy:
  - trusted hosts are now only local loopback and `commanddeck-web`.

## Data model / SQLC

- Updated upsert SQL so null runtime from sync does not erase existing proven runtime:
  - `runtime_id = COALESCE(EXCLUDED.runtime_id, preview_registry.runtime_id)`
- Updated generated SQLC query file to match.
- Migration `087_preview_registry` retained; FK/cascade/nullability constraints unchanged and validated by tests/build.

## Tests and verification

- Added/updated backend tests:
  - GET is read-only and does not create rows.
  - trusted sync persists exactly one self-hosted row and remains idempotent.
  - sync does not assign unproven runtime/machine provenance.
- Added/updated frontend tests:
  - explicit refresh button calls trusted sync endpoint only.
  - self-hosted preview renders unlinked runtime truthfully.
- Ran:
  - `scripts/doctor.ps1`
  - `pnpm.cmd lint`
  - `pnpm.cmd build`
  - `pnpm.cmd test`
  - Docker Go (`golang:1.26`) formatting + targeted Go tests/build (`./internal/handler`, `./cmd/server`)

## Product behavior

- Preview registry list is durable and read-only on polling.
- Registry mutation is explicit and visible in UI (`Register/Refresh Preview`).
- Safe unavailable messaging and redirect blocking preserved.
- Command-runner behavior and commanddeck run history UI remain intact.

## Next branch objective

`feature/commanddeck-preview-health-refresh-control-013`: build the next narrow visible preview control step from updated main.
