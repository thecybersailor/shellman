# How to Add a New Program Adapter

This guide is a short checklist for adding a new command adapter (like `codex` or `cursor-agent`) in Shellman.

## 1. Confirm Runtime Signature First

Do not guess. Start the real command in a tmux pane, then inspect runtime state.

- `pane_current_command` is often not enough (for wrapped CLIs it may only show `node`).
- Use runtime process data (`CurrentBinary`, `CurrentArgs`) as the primary signal.

In code, Shellman reads this from `tmux.Adapter.PaneRuntimeState(...)`.

## 2. Implement a New Detector Package

Create a package under:

- `cli/internal/progdetector/<your_program>/detector.go`

Implement `programadapter.ProgramAdapter`:

- `ProgramID() string`
- `IsAvailable(ctx)`  
  Use `programadapter.CommandExists(ctx, "<real-entry-command>")` (uses `which`).
- `MatchCurrentCommand(...)`  
  Keep as fallback compatibility.
- `MatchRuntimeState(state)`  
  Main logic: match by `CurrentBinary` + `CurrentArgs`.
- `HasExitedMode(ctx, state)`  
  Exit when runtime state no longer matches your program.
- `BuildInputPromptSteps(prompt)`  
  Define program-specific input + submit keys.

Register it in `init()` with:

- `progdetector.ProgramDetectorRegistry.MustRegister(New())`

Then add blank import in:

- `cli/internal/progdetector/builtin/init.go`

## 3. Enter/Exit Detection Rule

Recommended rule:

1. Try runtime signature first (`binary + args`).
2. If runtime signature is unavailable, fallback to `CurrentCommand`.
3. Keep enter/exit logic symmetric (same signature for both).

## 4. Tests (TDD)

Write tests before implementation changes:

1. Detector unit tests:
   - `MatchRuntimeState` positive and negative cases.
   - `HasExitedMode` enter/exit transitions.
   - `IsAvailable` canceled context behavior.
2. Realtime mode tests in localapi:
   - Ambiguous display command (for example `node`) but runtime args match your adapter.
   - Expect AI-agent mode and `task.input_prompt` tool availability.
3. Regression:
   - `go test ./internal/progdetector/...`
   - `go test ./internal/localapi ./cmd/shellman ./internal/projectstate`

## 5. No Storage Changes for Runtime Args

For adapter detection, pass runtime args only in realtime flow.  
Do not add DB fields unless explicitly confirmed by product requirements.
