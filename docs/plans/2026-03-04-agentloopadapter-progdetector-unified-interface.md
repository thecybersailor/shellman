# AgentLoopAdapter/ProgDetector Unified Interface Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Define a single shared program-adapter interface used by both mode detection and task input prompt behavior, while keeping `IsAvailable` and implementing it via `which`.

**Architecture:** Introduce a new shared contract package under `cli/internal/programadapter` and make `progdetector` consume this contract instead of defining its own interface/types. Keep behavior unchanged for mode enter/exit and prompt-step generation. Replace per-detector `exec.LookPath` checks with a shared `which`-based command probe to ensure `IsAvailable` is consistent.

**Tech Stack:** Go 1.24, standard library (`context`, `os/exec`, `errors`, `time`), existing `progdetector` registry/state machine, Go test.

**Execution Skills:** `@superpowers/test-driven-development`, `@superpowers/systematic-debugging`, `@superpowers/verification-before-completion`

---

### Task 1: Create Shared Program Adapter Contract Package

**Files:**
- Create: `cli/internal/programadapter/types.go`
- Create: `cli/internal/programadapter/command_probe.go`
- Test: `cli/internal/programadapter/command_probe_test.go`

**Step 1: Write the failing test**

```go
package programadapter

import (
    "context"
    "errors"
    "testing"
)

type fakeRunner struct {
    err error
}

func (f fakeRunner) Run(context.Context, string, ...string) error {
    return f.err
}

func TestCommandExists_UsesWhichAndHandlesOutcomes(t *testing.T) {
    ok, err := commandExistsWithRunner(context.Background(), "codex", fakeRunner{err: nil})
    if err != nil || !ok {
        t.Fatalf("expected command exists, got ok=%v err=%v", ok, err)
    }

    ok, err = commandExistsWithRunner(context.Background(), "codex", fakeRunner{err: errors.New("exit status 1")})
    if err != nil || ok {
        t.Fatalf("expected command not found without hard error, got ok=%v err=%v", ok, err)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `cd cli && go test ./internal/programadapter -run CommandExists_UsesWhichAndHandlesOutcomes -v`
Expected: FAIL because package/functions do not exist yet.

**Step 3: Write minimal implementation**

```go
package programadapter

import (
    "context"
    "os/exec"
    "strings"
    "time"
)

type RuntimeState struct {
    CurrentCommand string
    ViewportText   string
    CursorVisible  bool
}

type PromptStep struct {
    Input     string
    Delay     time.Duration
    TimeoutMs int
}

type ProgramAdapter interface {
    ProgramID() string
    IsAvailable(ctx context.Context) (bool, error)
    MatchCurrentCommand(currentCommand string) bool
    HasExitedMode(ctx context.Context, state RuntimeState) (bool, error)
    BuildInputPromptSteps(prompt string) ([]PromptStep, error)
}

type commandRunner interface {
    Run(ctx context.Context, name string, args ...string) error
}

type osExecRunner struct{}

func (osExecRunner) Run(ctx context.Context, name string, args ...string) error {
    return exec.CommandContext(ctx, name, args...).Run()
}

func CommandExists(ctx context.Context, command string) (bool, error) {
    return commandExistsWithRunner(ctx, command, osExecRunner{})
}

func commandExistsWithRunner(ctx context.Context, command string, runner commandRunner) (bool, error) {
    command = strings.TrimSpace(command)
    if command == "" {
        return false, nil
    }
    if err := runner.Run(ctx, "which", command); err != nil {
        return false, nil
    }
    return true, nil
}
```

**Step 4: Run test to verify it passes**

Run: `cd cli && go test ./internal/programadapter -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add cli/internal/programadapter/types.go cli/internal/programadapter/command_probe.go cli/internal/programadapter/command_probe_test.go
git commit -m "feat: add shared program adapter contract and which probe"
```

### Task 2: Switch ProgDetector to the Shared Interface Definition

**Files:**
- Modify: `cli/internal/progdetector/types.go`
- Modify: `cli/internal/progdetector/registry.go`
- Modify: `cli/internal/progdetector/registry_test.go`
- Modify: `cli/internal/progdetector/state_machine.go`

**Step 1: Write the failing test (compile-level contract check)**

```go
func TestRegistryRegisterGetDetect(t *testing.T) {
    // keep existing test, but fakeDetector must satisfy the shared ProgramAdapter contract
}
```

**Step 2: Run test to verify it fails**

Run: `cd cli && go test ./internal/progdetector -run Registry -v`
Expected: FAIL/compile error after changing `progdetector` to shared contract until fake detector is updated.

**Step 3: Write minimal implementation**

```go
// types.go
package progdetector

import "shellman/cli/internal/programadapter"

type RuntimeState = programadapter.RuntimeState
type PromptStep = programadapter.PromptStep
type Detector = programadapter.ProgramAdapter
```

```go
// registry.go (core field type updates)
type Registry struct {
    mu    sync.RWMutex
    byID  map[string]Detector
    order []string
}
```

**Step 4: Run test to verify it passes**

Run: `cd cli && go test ./internal/progdetector -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add cli/internal/progdetector/types.go cli/internal/progdetector/registry.go cli/internal/progdetector/registry_test.go cli/internal/progdetector/state_machine.go
git commit -m "refactor: make progdetector use shared program adapter interface"
```

### Task 3: Migrate Built-in Detectors to Shared `which` Availability

**Files:**
- Modify: `cli/internal/progdetector/codex/detector.go`
- Modify: `cli/internal/progdetector/claude/detector.go`
- Modify: `cli/internal/progdetector/cursor/detector.go`
- Modify: `cli/internal/progdetector/antigravity/detector.go`
- Test: `cli/internal/progdetector/codex/detector_test.go`

**Step 1: Write the failing test**

```go
func TestDetectorIsAvailable_EmptyProgramIDGuard(t *testing.T) {
    // add minimal guard coverage for availability helper via detector path
}
```

**Step 2: Run test to verify it fails**

Run: `cd cli && go test ./internal/progdetector/codex -run IsAvailable -v`
Expected: FAIL before implementation update.

**Step 3: Write minimal implementation**

```go
import "shellman/cli/internal/programadapter"

func (Detector) IsAvailable(ctx context.Context) (bool, error) {
    return programadapter.CommandExists(ctx, programID)
}
```

Apply the same pattern in all built-in detector packages.

**Step 4: Run test to verify it passes**

Run: `cd cli && go test ./internal/progdetector/... -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add cli/internal/progdetector/codex/detector.go cli/internal/progdetector/claude/detector.go cli/internal/progdetector/cursor/detector.go cli/internal/progdetector/antigravity/detector.go cli/internal/progdetector/codex/detector_test.go
git commit -m "refactor: use shared which-based IsAvailable for builtin detectors"
```

### Task 4: Verify Cross-Package Integration for Task-Agent Mode and Input Prompt

**Files:**
- Modify: `cli/cmd/shellman/main.go` (only if type imports need adjustment)
- Test: `cli/cmd/shellman/main_spawn_tool_test.go`
- Test: `cli/internal/localapi/task_agent_actor_mode_test.go`
- Test: `cli/internal/projectstate/task_state_store_test.go`

**Step 1: Write/adjust failing tests (if compile or behavior breaks)**

```go
func TestBuildInputPromptStepsForCommand_Codex(t *testing.T) {
    // existing expectations must remain unchanged
}
```

**Step 2: Run test to verify failure point**

Run: `cd cli && go test ./cmd/shellman ./internal/localapi ./internal/projectstate -run "BuildInputPromptStepsForCommand|resolveTaskAgentToolModeAndNames|active_adapter" -v`
Expected: If integration type mismatch exists, FAIL reveals exact break.

**Step 3: Write minimal implementation**

- Keep runtime behavior unchanged.
- Only update imports/types to shared aliases where necessary.
- Do not add new routes, protocols, or fallback paths.

**Step 4: Run test to verify it passes**

Run: `cd cli && go test ./cmd/shellman ./internal/localapi ./internal/projectstate -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add cli/cmd/shellman/main.go cli/cmd/shellman/main_spawn_tool_test.go cli/internal/localapi/task_agent_actor_mode_test.go cli/internal/projectstate/task_state_store_test.go
git commit -m "test: verify unified adapter contract keeps task-agent behavior unchanged"
```

### Task 5: Keep Design Doc in Sync

**Files:**
- Create: `docs/design/program-adapter-contract.md`

**Step 1: Write the doc update**

```md
# Program Adapter Contract

- Unified interface location: `cli/internal/programadapter/types.go`
- `IsAvailable` uses `which <program>` via shared helper.
- Built-in detector packages implement only adapter-specific behavior.
```

**Step 2: Run validation checks**

Run: `cd cli && go test ./internal/progdetector ./cmd/shellman ./internal/localapi ./internal/projectstate -v`
Expected: PASS.

**Step 3: Commit**

```bash
git add docs/design/program-adapter-contract.md
git commit -m "docs: add unified program adapter contract notes"
```

### Final Verification Gate

**Step 1: Full targeted verification**

Run:

```bash
cd cli && go test ./internal/programadapter ./internal/progdetector ./internal/localapi ./internal/projectstate ./cmd/shellman -v
```

Expected: PASS all.

**Step 2: Optional e2e reminder (do not run locally)**

- If e2e is required later, run it in Docker only.
- Keep this plan limited to unit/integration scope.

**Step 3: Branch status check**

Run: `git status --short --branch`
Expected: clean tree on `feat/unify-adapter-interface`.
