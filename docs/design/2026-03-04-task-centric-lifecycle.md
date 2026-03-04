# Task-Centric Lifecycle (Run Layer Removed)

Date: 2026-03-04

## Decision

Shellman removes the run domain model and keeps one execution path:

- Task lifecycle status is the only completion state.
- Pane binding is the only runtime execution identity.
- APIs/events/prompts do not carry `run_id`.

## State Model

Entity: `task`

- `pending`: task created but not started.
- `running`: task currently executing.
- `waiting_user`: task paused and waiting user action.
- `waiting_children`: parent waits for child tasks.
- `completed|failed|canceled`: terminal states.

Entity: `pane binding`

- `task_id <-> pane_id/pane_target` binds runtime terminal identity to a task.
- Binding is independent from task terminal status, but completion flow reads binding to determine which task to update.

## Trigger Model

Start triggers:

- create/adopt pane -> bind pane to task
- task marked running by orchestrator

Completion triggers:

- explicit `POST /api/v1/tasks/:id/report-result`
- pane auto-complete detector (`AutoCompleteByPane`) resolves task by bound pane and completes task

## API Contract Changes

- Removed routes:
  - `POST /api/v1/tasks/:id/runs`
  - `/api/v1/runs/*`
- Removed response/event field:
  - `run_id`
- Child spawn and task completion payloads are task-centric only.

## Persistence Changes

Removed run tables:

- `task_runs`
- `run_bindings`
- `run_events`
- `completion_inbox`
- `action_outbox`

Kept task-centric storage:

- `tasks`
- `task_notes`
- `task_messages`
- `task_runtime`
- `pane_runtime`
- `pane_snapshots`
- `pane_output_state`

## Operational Implications

- Fewer state transitions and no run/task dual source of truth.
- Lower contract ambiguity in backend and webui.
- Migration path is hard-cut only (no compatibility fallback).
