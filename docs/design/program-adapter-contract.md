# Program Adapter Contract

## Purpose

This document defines the single adapter contract shared by task-agent mode detection and `task.input_prompt` step generation.

## Contract Location

- `cli/internal/programadapter/types.go`

## ProgramAdapter Interface

Each built-in adapter and future adapter must implement:

- `ProgramID() string`
- `IsAvailable(ctx context.Context) (bool, error)`
- `MatchCurrentCommand(currentCommand string) bool`
- `HasExitedMode(ctx context.Context, state RuntimeState) (bool, error)`
- `BuildInputPromptSteps(prompt string) ([]PromptStep, error)`

## Shared Types

- `RuntimeState`
- `PromptStep`

`cli/internal/progdetector/types.go` re-exports these as aliases, so existing callers keep one import path while using the shared contract identity.

## Availability Rule

`IsAvailable` must use `which <program>` through:

- `cli/internal/programadapter/command_probe.go`
- `programadapter.CommandExists(ctx, programID)`

Behavior:

- `which` exit status means command not found: return `(false, nil)`.
- other execution errors are returned as hard errors.
- canceled context is propagated as `context.Canceled`.

## Non-Goals

- No new API routes.
- No fallback compatibility path for legacy adapter interfaces.
- No behavior change in `ResolveActiveAdapter` state-machine semantics.
