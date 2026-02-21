# TS Type Auto-Mapping Scope

## Source Of Truth

- Single source of truth for webui API contract types: Go struct definitions mapped by `cli/cmd/gen-api-types`.
- Generated output: `webui/src/generated/api-types.ts`.

## In Scope

- `cli/internal/localapi` HTTP JSON contract models.
- Contract models imported by localapi from:
  - `cli/internal/projectstate`
  - `cli/internal/global`
  - `cli/internal/fsbrowser`

## Out Of Scope

- WebSocket event payloads.
- Edge worker internal protocol.
- API client generation.
- Any pnpm-based generation pipeline.

## Rules

- Generated file is read-only for manual edits.
- No duplicate hand-written API contract type if generated equivalent exists.
- Validation must fail when generated output drifts.
