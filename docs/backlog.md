# Backlog

## 2026-02-22 - task pane binding collision when using child spawn

- Status: open
- Priority: high
- Scope: `child.spawn` / task switch / pane binding

### Symptom

- `Plan-03` and `Plan-04` had different `pane_uuid`, but clicking both tasks showed the same pane content.
- Repro condition observed in production logs: both tasks created by `child.spawn`.

### Root Contradiction (from logs and DB)

- Two task bindings existed with different `pane_uuid`.
- Both live runtime bindings converged to the same `pane_target` (`botworks:9.1`).
- `pane_target` was based on `session:window.pane_index`, which is not stable after tmux layout/window changes.

### Decision

- Replace unstable pane target source with tmux global pane identity `%pane_id`.
- Keep one path: create/list/rebind/stream target identity should use `%pane_id`.

### Acceptance

- New pane creation APIs return `%pane_id` in `pane_id` and `pane_target`.
- Selecting different tasks created via `child.spawn` no longer collapses to one pane due to target reuse.
- Control-mode routing resolves `%pane_id` to session and keeps output correctly routed.
