# TS Type Auto-Mapping

## Commands

- Generate API TS types:
  - `make gen-api-types`
- Validate API type drift:
  - `make validate-api-types`

## Workflow

1. Change Go API contract structs.
2. Run `make gen-api-types`.
3. Update webui code to use generated types from `@/generated/api-types`.
4. Run `make validate-quick`.

## Guardrails

- `webui/src/generated/api-types.ts` is generated and read-only.
- Keep single source of truth in Go structs.
- Do not add pnpm command path for type generation.
