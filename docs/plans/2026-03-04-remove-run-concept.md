# Remove Run Concept Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove the `run` domain model entirely and make `task + pane binding` the only execution state model.

**Architecture:** We hard-cut from run-centric flow to task-centric flow. Task lifecycle (`pending/running/.../completed`) becomes the only completion state, while pane identity stays in `PanesIndex` and runtime snapshots stay in `pane_runtime/task_runtime`. All APIs/events/prompts stop carrying `run_id`, and backend persistence removes run tables and run store methods.

**Tech Stack:** Go (localapi, projectstate, db/gorm/sqlite), Vue + Pinia + Vitest, Docker Compose e2e.

---

## Approach Comparison

1. **Hard cut (Recommended)**
- Remove all run routes/models/store APIs in one branch.
- Enforce single-path architecture immediately.
- Matches project rule: no legacy compatibility path.

2. **Two-phase compatibility**
- Keep run APIs internally and add task-centric APIs first, then remove run later.
- Lower short-term risk, but violates single-path rule and extends complexity.

3. **Storage-only removal first**
- Keep route/output `run_id` but fake values and remove DB writes.
- Fast compile pass, but semantics become misleading and brittle.

**Recommendation:** Use Approach 1 (hard cut).

---

### Task 1: Remove Run HTTP Surface

**Files:**
- Modify: `cli/internal/localapi/server.go`
- Modify: `cli/internal/localapi/routes_tasks.go`
- Delete: `cli/internal/localapi/routes_runs.go`
- Modify: `cli/internal/localapi/routes_runs_test.go`

**Step 1: Write failing tests**

```go
// routes_runs_test.go
func TestRunRoutes_Removed_Return404(t *testing.T) {
  // POST /api/v1/tasks/:id/runs => 404
  // /api/v1/runs/* => 404
}
```

**Step 2: Run test to verify it fails**

Run: `cd cli && go test ./internal/localapi -run TestRunRoutes_Removed_Return404 -count=1`
Expected: FAIL because run routes are still registered.

**Step 3: Write minimal implementation**

```go
// server.go
// remove s.registerRunRoutes()

// routes_tasks.go
// remove case: POST .../runs
```

Delete `routes_runs.go` entirely.

**Step 4: Run test to verify it passes**

Run: `cd cli && go test ./internal/localapi -run TestRunRoutes_Removed_Return404 -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add cli/internal/localapi/server.go cli/internal/localapi/routes_tasks.go cli/internal/localapi/routes_runs_test.go cli/internal/localapi/routes_runs.go
git commit -m "refactor(localapi): remove run routes"
```

---

### Task 2: Remove `run_id` From Pane Create/Adopt Contract

**Files:**
- Modify: `cli/internal/localapi/routes_panes.go`
- Modify: `cli/internal/localapi/routes_panes_test.go`
- Modify: `cli/internal/localapi/routes_tasks.go`

**Step 1: Write failing tests**

```go
// routes_panes_test.go
// assert pane create/adopt response has task_id/pane_id/pane_target only
// assert no run_id in response payload
```

**Step 2: Run test to verify it fails**

Run: `cd cli && go test ./internal/localapi -run 'TestPaneCreationRoutes|TestAdoptPane' -count=1`
Expected: FAIL due existing `run_id` assertions/fields.

**Step 3: Write minimal implementation**

```go
// routes_panes.go
// remove createRunAndLiveBinding(...)
// remove run_id from response + pane.created event payload
```

**Step 4: Run test to verify it passes**

Run: `cd cli && go test ./internal/localapi -run 'TestPaneCreationRoutes|TestAdoptPane' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add cli/internal/localapi/routes_panes.go cli/internal/localapi/routes_panes_test.go cli/internal/localapi/routes_tasks.go
git commit -m "refactor(localapi): remove run_id from pane lifecycle contract"
```

---

### Task 3: Convert Auto-Complete Flow To Task-Centric

**Files:**
- Create: `cli/internal/localapi/pane_task_lookup.go`
- Modify: `cli/internal/localapi/run_autocomplete.go`
- Modify: `cli/internal/localapi/task_completion.go`
- Modify: `cli/internal/localapi/task_agent_prompt.go`
- Modify: `cli/cmd/shellman/status_pump.go`
- Modify: `cli/internal/localapi/routes_runs_report_result_test.go`
- Modify: `cli/internal/localapi/task_agent_prompt_test.go`
- Modify: `cli/cmd/shellman/status_pump_test.go`

**Step 1: Write failing tests**

```go
// run_autocomplete/task completion tests
// expect completion path emits task.return.reported without run_id
// expect no branch for no-live-running-run
```

**Step 2: Run test to verify it fails**

Run: `cd cli && go test ./internal/localapi ./cmd/shellman -run 'AutoCompleteByPane|TaskAgentAutoProgressPrompt|StatusPump' -count=1`
Expected: FAIL on old run-centric assertions.

**Step 3: Write minimal implementation**

```go
// run_autocomplete.go
// resolve task by pane, complete task directly, enqueue task completion actions

// task_completion.go
// remove completeRunAndEnqueueActions + enqueueRunCompletionActions

// task_agent_prompt.go
// remove RunID field and run_id meta emission
```

**Step 4: Run test to verify it passes**

Run: `cd cli && go test ./internal/localapi ./cmd/shellman -run 'AutoCompleteByPane|TaskAgentAutoProgressPrompt|StatusPump' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add cli/internal/localapi/pane_task_lookup.go cli/internal/localapi/run_autocomplete.go cli/internal/localapi/task_completion.go cli/internal/localapi/task_agent_prompt.go cli/cmd/shellman/status_pump.go cli/internal/localapi/routes_runs_report_result_test.go cli/internal/localapi/task_agent_prompt_test.go cli/cmd/shellman/status_pump_test.go
git commit -m "refactor(runtime): make auto-complete task-centric"
```

---

### Task 4: Remove Run Persistence Model and DB Schema

**Files:**
- Modify: `cli/internal/db/models.go`
- Modify: `cli/internal/db/migrations.go`
- Modify: `cli/internal/db/migrations_test.go`
- Modify: `cli/internal/projectstate/run_types.go`
- Modify: `cli/internal/projectstate/run_store.go`
- Modify: `cli/internal/projectstate/run_store_test.go`
- Modify: `cli/internal/projectstate/schema_v1_runcentric_test.go`

**Step 1: Write failing tests**

```go
// migrations_test.go / schema_v1_runcentric_test.go
// assert run tables are absent: task_runs/run_bindings/run_events/completion_inbox/action_outbox
```

**Step 2: Run test to verify it fails**

Run: `cd cli && go test ./internal/db ./internal/projectstate -run 'Migrations|RunCentricSchema' -count=1`
Expected: FAIL because run tables currently exist.

**Step 3: Write minimal implementation**

```go
// models.go: delete TaskRun/RunBinding/RunEvent/CompletionInbox/ActionOutbox
// migrations.go: remove automigrate entries + remove run indexes
// run_store.go/run_types.go: delete run-only constants/structs/methods
```

If existing DB cleanup is required, add explicit `DROP TABLE IF EXISTS ...` in migration path and cover with tests.

**Step 4: Run test to verify it passes**

Run: `cd cli && go test ./internal/db ./internal/projectstate -run 'Migrations|RunCentricSchema|RunStore' -count=1`
Expected: PASS with task-centric schema.

**Step 5: Commit**

```bash
git add cli/internal/db/models.go cli/internal/db/migrations.go cli/internal/db/migrations_test.go cli/internal/projectstate/run_types.go cli/internal/projectstate/run_store.go cli/internal/projectstate/run_store_test.go cli/internal/projectstate/schema_v1_runcentric_test.go
git commit -m "refactor(projectstate): remove run persistence model"
```

---

### Task 5: Remove Run Contract From Tooling and WebUI Store

**Files:**
- Modify: `cli/cmd/shellman/main.go`
- Modify: `cli/cmd/shellman/main_spawn_tool_test.go`
- Modify: `webui/src/stores/shellman.ts`
- Modify: `webui/src/stores/shellman.spec.ts`

**Step 1: Write failing tests**

```ts
// shellman.spec.ts
// remove reportRunResult expectations
// assert store API no longer exposes reportRunResult
```

```go
// main_spawn_tool_test.go
// child spawn response fixture without run_id; output must not emit run_id
```

**Step 2: Run test to verify it fails**

Run: `cd cli && go test ./cmd/shellman -run TestExecuteTaskChildSpawnAction_AutoEnterSidecarModeAndPrompt -count=1`
Run: `cd webui && npm test -- src/stores/shellman.spec.ts`
Expected: FAIL on legacy run_id/reportRunResult contract.

**Step 3: Write minimal implementation**

```go
// main.go: remove RunID decode/output in executeTaskChildSpawnAction
```

```ts
// shellman.ts: remove reportRunResult() and export
```

**Step 4: Run test to verify it passes**

Run: `cd cli && go test ./cmd/shellman -run TestExecuteTaskChildSpawnAction_AutoEnterSidecarModeAndPrompt -count=1`
Run: `cd webui && npm test -- src/stores/shellman.spec.ts`
Expected: PASS.

**Step 5: Commit**

```bash
git add cli/cmd/shellman/main.go cli/cmd/shellman/main_spawn_tool_test.go webui/src/stores/shellman.ts webui/src/stores/shellman.spec.ts
git commit -m "refactor(contract): remove run_id from tool and web store APIs"
```

---

### Task 6: End-to-End Verification and Documentation Sync

**Files:**
- Modify: `README.md`
- Modify: `docs/api/type-mapping.md`
- Modify: `docs/api/type-mapping-scope.md`
- Create: `docs/design/2026-03-04-task-centric-lifecycle.md`

**Step 1: Write/update docs for the new single-path model**

```md
- Remove run terminology from API and lifecycle docs
- Define task status + pane binding as the only execution model
```

**Step 2: Run backend/frontend full tests**

Run: `cd cli && go test ./...`
Run: `cd webui && npm test`
Expected: PASS.

**Step 3: Run required docker e2e**

Run: `cd . && make e2e-ui-docker`
Expected: PASS in clean docker environment.

**Step 4: Sanity check git diff scope**

Run: `git status --short`
Expected: only files in this plan scope changed.

**Step 5: Commit**

```bash
git add README.md docs/api/type-mapping.md docs/api/type-mapping-scope.md docs/design/2026-03-04-task-centric-lifecycle.md
git commit -m "docs: align architecture docs with task-centric execution model"
```

---

## Final Validation Checklist

Run in order:

```bash
cd cli && go test ./...
cd ../webui && npm test
cd .. && make e2e-ui-docker
```

Expected:
- No run route remains.
- No `run_id` in task/pane API contracts or task-agent prompt meta.
- DB schema and store code contain no run tables/models/methods.
- Task completion remains event-driven and stable.

