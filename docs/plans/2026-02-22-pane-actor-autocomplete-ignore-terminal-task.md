# Pane-Actor Auto-Complete Ignore Terminal Task Gate Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ensure pane-actor/obs auto-complete is controlled by sidecar mode and observed timestamp de-dup only, not blocked by task terminal status or task flag.

**Architecture:** Keep existing auto-complete pipeline and sidecar/advisor guard unchanged, but remove the `task-already-terminal` short-circuit in `AutoCompleteByPane`. Validate behavior with focused localapi tests: completed task + pane-actor should still trigger, while advisor and duplicate-observed protections remain intact.

**Tech Stack:** Go, net/http local API handlers, existing projectstate store, `go test`.

---

### Task 1: Add failing test for expected new behavior (TDD first)

**Files:**
- Modify: `shellman/cli/internal/localapi/routes_runs_report_result_test.go`
- Test: `shellman/cli/internal/localapi/routes_runs_report_result_test.go`

**Step 1: Replace terminal-skip expectation test with trigger expectation**

Add/update a test to verify: when trigger source is `pane-actor` and task is already completed, `AutoCompleteByPane` still returns `Triggered=true`.

```go
func TestAutoCompleteByPane_PaneActorTriggersWhenTaskAlreadyCompleted(t *testing.T) {
    srv := newTestServer(t)

    // Setup project/task/pane binding and mark task as completed first.
    // Reuse existing helper patterns from routes_runs_report_result_test.go.

    out, runErr := srv.AutoCompleteByPane(AutoCompleteByPaneInput{
        PaneTarget:           paneTarget,
        TriggerSource:        "pane-actor",
        ObservedLastActiveAt: observed,
    })
    if runErr != nil {
        t.Fatalf("AutoCompleteByPane failed: %v", runErr)
    }
    if !out.Triggered {
        t.Fatalf("expected triggered=true for pane-actor on terminal task, got false (status=%q reason=%q)", out.Status, out.Reason)
    }
    if out.Reason != "" {
        t.Fatalf("expected empty reason on success, got %q", out.Reason)
    }
}
```

**Step 2: Run the single test to confirm it fails**

Run: `cd shellman && go test ./cli/internal/localapi -run TestAutoCompleteByPane_PaneActorTriggersWhenTaskAlreadyCompleted -count=1`
Expected: FAIL (current code returns `reason=task-already-terminal` and `Triggered=false`).

**Step 3: Commit failing test scaffolding**

```bash
cd shellman
git add cli/internal/localapi/routes_runs_report_result_test.go
git commit -m "test: define pane-actor autocomplete behavior for terminal task"
```

### Task 2: Remove terminal-status short-circuit in autocomplete flow

**Files:**
- Modify: `shellman/cli/internal/localapi/run_autocomplete.go`
- Test: `shellman/cli/internal/localapi/routes_runs_report_result_test.go`

**Step 1: Delete terminal-task skip block in `AutoCompleteByPane`**

Remove this guard:

```go
if strings.EqualFold(triggerSource, "pane-actor") && isTaskTerminalStatus(taskEntry.Status) {
    return AutoCompleteByPaneResult{
        Triggered:  false,
        PaneTarget: paneTarget,
        Reason:     "task-already-terminal",
        RunID:      "",
        TaskID:     taskID,
        Status:     strings.TrimSpace(taskEntry.Status),
    }, nil
}
```

Keep existing behavior unchanged for:
- `sidecar-mode-advisor` skip
- `duplicate-observed-last-active-at` skip
- optional run lookup + completion enqueue logic

**Step 2: Align/rename old test symbol(s)**

If existing test function name still implies skip behavior, rename to trigger behavior name and ensure assertions match Task 1.

**Step 3: Run focused test set**

Run:
- `cd shellman && go test ./cli/internal/localapi -run TestAutoCompleteByPane_PaneActorTriggersWhenTaskAlreadyCompleted -count=1`
- `cd shellman && go test ./cli/internal/localapi -run TestRunAutoCompleteByPane_SkipsOnlyPaneActorSourceWhenSidecarModeAdvisor -count=1`
- `cd shellman && go test ./cli/internal/localapi -run TestRunAutoCompleteByPane_DedupesPaneActorByObservedLastActiveAt -count=1`

Expected:
- terminal-task pane-actor test PASS (now triggers)
- advisor skip test still PASS
- observed de-dup test still PASS

**Step 4: Commit behavior change**

```bash
cd shellman
git add cli/internal/localapi/run_autocomplete.go cli/internal/localapi/routes_runs_report_result_test.go
git commit -m "fix: decouple pane-actor autocomplete from task terminal status"
```

### Task 3: Regression sweep for localapi package

**Files:**
- Test only: `shellman/cli/internal/localapi/*`

**Step 1: Run package-level localapi tests**

Run: `cd shellman && go test ./cli/internal/localapi -count=1`
Expected: PASS without introducing new failures.

**Step 2: Sanity grep for removed reason branch**

Run: `cd shellman && rg -n "task-already-terminal" cli/internal/localapi`
Expected: no runtime usage in `run_autocomplete.go`; only historical references if any are intentionally removed/updated.

**Step 3: Commit final cleanup if needed**

```bash
cd shellman
git add -A
git commit -m "test: keep autocomplete invariants for advisor and observed dedupe"
```

