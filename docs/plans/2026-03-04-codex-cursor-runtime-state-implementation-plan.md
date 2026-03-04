# Codex Cursor Runtime State Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make Codex and Cursor detector decisions use realtime foreground process binary/args (not only display command), without any storage changes.

**Architecture:** Extend runtime state contract with process binary/args, add tmux realtime process snapshot API, route localapi realtime mode resolution through state-based detector APIs, and update codex/cursor detector matching logic with strict fallback to command-only behavior when runtime args are unavailable.

**Tech Stack:** Go 1.24, gopsutil process APIs already in tmux adapter, existing progdetector registry/state machine, go test.

---

### Task 1: Extend Runtime Contract and Registry State APIs

**Files:**
- Modify: `cli/internal/programadapter/types.go`
- Modify: `cli/internal/progdetector/registry.go`
- Modify: `cli/internal/progdetector/state_machine.go`
- Modify: `cli/internal/progdetector/registry_test.go`

**Step 1: Write failing tests**
- Add state-based registry/state-machine tests asserting detection can use `CurrentBinary` and `CurrentArgs`.

**Step 2: Run red tests**
Run: `cd cli && go test ./internal/progdetector -run "State|Registry" -v`
Expected: FAIL for missing state-based APIs.

**Step 3: Minimal implementation**
- Add `CurrentBinary` and `CurrentArgs` to `RuntimeState`.
- Add `DetectByState` and `ResolveActiveAdapterByState`.
- Keep command-only entry points as wrappers.

**Step 4: Run green tests**
Run: `cd cli && go test ./internal/progdetector -v`
Expected: PASS.

### Task 2: Provide Realtime Process Binary/Args from tmux Adapter

**Files:**
- Modify: `cli/internal/tmux/adapter.go`
- Modify: `cli/internal/tmux/adapter_test.go`

**Step 1: Write failing tests**
- Add tests for helper/runtime parsing to ensure binary/args extraction for node-based wrappers (codex/cursor-agent style argv).

**Step 2: Run red tests**
Run: `cd cli && go test ./internal/tmux -run "Process|Runtime" -v`
Expected: FAIL before extraction API/helpers exist.

**Step 3: Minimal implementation**
- Add adapter runtime snapshot API returning command+binary+args.
- Keep existing command API behavior unchanged.

**Step 4: Run green tests**
Run: `cd cli && go test ./internal/tmux -v`
Expected: PASS.

### Task 3: Update Codex/Cursor Detector Matching to RuntimeState

**Files:**
- Modify: `cli/internal/progdetector/codex/detector.go`
- Modify: `cli/internal/progdetector/cursor/detector.go`
- Modify: `cli/internal/progdetector/codex/detector_test.go`
- Create: `cli/internal/progdetector/cursor/detector_test.go`

**Step 1: Write failing tests**
- Codex: `node + args(...codex...)` should match and remain in-mode.
- Cursor: `node + args(...cursor-agent...)` should match and remain in-mode.
- Non-matching node args should exit.

**Step 2: Run red tests**
Run: `cd cli && go test ./internal/progdetector/codex ./internal/progdetector/cursor -v`
Expected: FAIL before runtime-state matching logic.

**Step 3: Minimal implementation**
- Implement runtime-state-aware matching and exit checks for codex/cursor.
- Keep command fallback when runtime process fields are empty.

**Step 4: Run green tests**
Run: `cd cli && go test ./internal/progdetector/... -v`
Expected: PASS.

### Task 4: Wire Realtime localapi Path to State-Based Detector APIs

**Files:**
- Modify: `cli/internal/localapi/task_completion.go`
- Modify: `cli/internal/localapi/task_agent_actor.go`
- Modify: `cli/internal/localapi/task_agent_actor_mode_test.go`

**Step 1: Write failing tests**
- Add realtime mode test cases where display command is ambiguous but runtime binary/args imply codex/cursor, expecting ai-agent mode and `task.input_prompt` tool.

**Step 2: Run red tests**
Run: `cd cli && go test ./internal/localapi -run "ModeAndNamesRealtime|ResolveTaskAgentToolMode" -v`
Expected: FAIL before realtime runtime-state wiring.

**Step 3: Minimal implementation**
- Add realtime pane runtime-state detection method.
- Use `ResolveActiveAdapterByState` in realtime mode resolution.
- Keep DB writes unchanged (`current_command`, `active_adapter`).

**Step 4: Run green tests**
Run: `cd cli && go test ./internal/localapi -v`
Expected: PASS.

### Task 5: Full Verification

**Files:**
- Modify (if needed): `docs/design/program-adapter-contract.md`

**Step 1: Run full target verification**
Run:
`cd cli && go test ./internal/tmux ./internal/programadapter ./internal/progdetector ./internal/localapi ./internal/projectstate ./cmd/shellman -v`
Expected: PASS.

**Step 2: Confirm no storage schema changes**
Run:
`cd cli && rg -n "current_command_args|binary_args|runtime_args" internal/db internal/projectstate`
Expected: no new persistence fields.
