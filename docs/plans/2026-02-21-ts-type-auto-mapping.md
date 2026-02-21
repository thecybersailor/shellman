# Shellman TS Type Auto-Mapping Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a single-source pipeline that generates TS types from backend API schema and replaces duplicated hand-written webui API types.

**Architecture:** Use one-way generation: Go/localapi contracts -> OpenAPI spec -> generated TS types only. Keep HTTP JSON scope only. Do not generate API client and do not keep dual type definitions in webui.

**Tech Stack:** Go (cli/localapi), npm scripts, OpenAPI generator tooling, TypeScript (webui), Makefile validation chain.

---

### Task 1: Freeze scope and source of truth

**Files:**
- Modify: `docs/plans/2026-02-21-ts-type-auto-mapping.md`
- Create: `docs/api/type-mapping-scope.md`

**Step 1: Write mapping scope document**

- Define exact in-scope domain: `cli` localapi HTTP JSON routes only.
- Define exact out-of-scope domain: WebSocket payloads, edge worker internal protocol, API client generation.

**Step 2: Declare single source of truth**

- Declare OpenAPI artifact as the only source for webui API types.
- Declare duplicated hand-written webui API contracts as temporary migration targets to delete.

**Step 3: Verification**

Run: `rg -n "source of truth|out-of-scope|HTTP JSON" docs/api/type-mapping-scope.md`  
Expected: all scope markers present.

### Task 2: Add backend schema generation entrypoint

**Files:**
- Modify: `cli/Makefile` (or current cli build script entrypoint file)
- Create: `cli/scripts/gen_openapi.sh`
- Create: `cli/tmp/openapi/localapi.openapi.json` (generated artifact path, do not hand edit)

**Step 1: Add deterministic generation command**

- Add one npm/make-callable command to generate OpenAPI JSON from Go localapi contract.
- Pin output path to `cli/tmp/openapi/localapi.openapi.json`.

**Step 2: Add non-empty artifact check**

- Generation command must fail if output file missing or empty.

**Step 3: Verification**

Run: `cd cli && <openapi-generate-command>`  
Expected: `cli/tmp/openapi/localapi.openapi.json` created and non-empty.

### Task 3: Add webui TS type generation entrypoint

**Files:**
- Modify: `webui/package.json`
- Create: `webui/scripts/gen-api-types.mjs`
- Create: `webui/src/generated/api-types.ts` (generated file, do not hand edit)

**Step 1: Add generation script**

- Read OpenAPI artifact from `../cli/tmp/openapi/localapi.openapi.json`.
- Generate TS types only into `webui/src/generated/api-types.ts`.

**Step 2: Add npm script command**

- Add `npm run gen:api-types` in `webui/package.json`.
- Ensure command fails fast when OpenAPI input does not exist.

**Step 3: Verification**

Run: `cd webui && npm run gen:api-types`  
Expected: `webui/src/generated/api-types.ts` generated successfully.

### Task 4: Wire generation chain at root level

**Files:**
- Modify: `Makefile`

**Step 1: Add one-chain command**

- Add root target like `gen-api-types`:
  - first generate OpenAPI in `cli`
  - then generate TS types in `webui`

**Step 2: Keep npm-only frontend commands**

- Use npm script entry only; do not add pnpm command path.

**Step 3: Verification**

Run: `make gen-api-types`  
Expected: both artifacts regenerated in sequence without manual step.

### Task 5: Replace hand-written webui API contract types

**Files:**
- Modify: `webui/src/stores/shellman.ts`
- Modify: `webui/src/App.vue`
- Modify: `webui/src/components/ProjectInfoPanel.vue`
- Modify: `webui/src/components/SCMPanel.vue`
- Modify: other files currently declaring duplicated localapi contracts

**Step 1: Identify duplicated contract types**

Run: `rg -n "interface .*Task|interface .*Project|/api/v1/" webui/src`  
Expected: locate hand-written API shapes.

**Step 2: Replace imports to generated types**

- Import from `@/generated/api-types` (or project alias equivalent).
- Keep UI-only view models local; replace only API contract types.

**Step 3: Remove dead duplicated types**

- Delete local duplicated API type blocks after replacement.

**Step 4: Verification**

Run: `cd webui && npm test`  
Expected: tests pass with generated type usage.

### Task 6: Add validation targets with status cache

**Files:**
- Create: `Makefile.validate`
- Modify: `Makefile`
- Create: `scripts/validate-e2e-entry-guard.sh`

**Step 1: Add minimal validate targets**

- `validate-api-types`: run full generation chain and fail on dirty diff.
- `validate-quick`: include API type validation + existing fast tests.
- `validate-all`: include `validate-quick` + existing full checks.
- `validate-clean`, `validate-status`: maintain `.build-status` cache.

**Step 2: Add e2e docker entry guard**

- Scan `docs/plans` for forbidden direct e2e execution text.
- Only allow docker entrypoint (`make e2e-docker`).

**Step 3: include validation makefile**

- In root `Makefile`, include `Makefile.validate`.

**Step 4: Verification**

Run: `make validate-quick`  
Expected: pass with deterministic outputs and no untracked generated drift.

### Task 7: Documentation synchronization

**Files:**
- Create: `docs/api/type-mapping.md`
- Modify: `docs/api/type-mapping-scope.md`
- Modify: any root developer docs that mention validation flow

**Step 1: Document developer workflow**

- Write exact commands for local regeneration and validation.

**Step 2: Document guardrails**

- Generated files are read-only.
- No manual duplicated API type definition allowed in webui.

**Step 3: Verification**

Run: `rg -n "gen-api-types|validate-api-types|read-only|single source" docs`  
Expected: all operational rules documented.

### Task 8: End-to-end verification before completion

**Files:**
- No new code files

**Step 1: Run verification command set**

Run:
- `make gen-api-types`
- `make validate-quick`
- `make e2e-docker`

Expected:
- type generation stable
- quick validation pass
- e2e docker path pass (no local direct e2e dependency)

**Step 2: Check generated drift**

Run: `git status --short`  
Expected: no unexpected generated changes after validation rerun.

**Step 3: Prepare change summary**

- List generated contract coverage and replaced webui type surfaces.
- List known residual risks (if any WS path remains hand-maintained by design).
