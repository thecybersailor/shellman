# Codex/Cursor Runtime State Detection Design

## Goal

Enable accurate detector decisions for Codex and Cursor using real foreground process runtime data (`binary + args`) from tmux panes, without adding any persistence fields.

## Scope

- In scope:
  - Realtime chain only (`localapi` runtime detection path).
  - `codex` and `cursor` detectors consume runtime process state.
  - `RuntimeState` extends with `CurrentBinary` and `CurrentArgs`.
- Out of scope:
  - Database schema/storage changes.
  - Non-realtime persistence of process args.
  - Behavior changes for `claude` and `antigravity` detectors.

## Current Problem

`pane_current_command` and current derived command are display-oriented and can be non-canonical for wrapped Node CLIs.

Observed in tmux probes:
- `cursor-agent` interactive run uses foreground process `node .../cursor-agent ... index.js`.
- `pane_current_command` is `node`.
- existing derived command can become `index`.

As a result, detector enter/exit based on display command only is imprecise.

## Design

1. Extend `programadapter.RuntimeState`
- Add `CurrentBinary string`.
- Add `CurrentArgs []string`.
- Keep existing `CurrentCommand` for backward-compatible display/fallback logic.

2. Add realtime pane process snapshot in `tmux.Adapter`
- New adapter API returns:
  - display command
  - process binary
  - process args
- Data source is foreground process PID/TPGID and process argv.
- No DB writes for binary/args.

3. Add state-based detection entry in `progdetector`
- Registry supports detection by full `RuntimeState`.
- State machine supports resolving active adapter by full `RuntimeState`.
- Existing command-only APIs remain as wrappers for non-realtime callers.

4. Codex/Cursor detector precision
- `codex`: match runtime state when binary is `node` and args contain Codex entry (`codex`), or binary itself is `codex`.
- `cursor`: match runtime state when binary is `node` and args contain `cursor-agent`, or binary itself is `cursor-agent`.
- If runtime binary/args are missing, fallback to current command matching.

5. Realtime localapi chain update
- `resolveTaskAgentToolModeAndNamesRealtime` uses pane runtime state (`command + binary + args`) for `ResolveActiveAdapterByState`.
- Persisted task metadata remains unchanged (`current_command`, `active_adapter` only).

## Validation Strategy (TDD)

- Red/Green tests for runtime-state matching in `codex` and `cursor` detectors.
- Red/Green tests for state-machine resolution using runtime state.
- Realtime mode tests to ensure `task.input_prompt` appears when runtime state indicates Codex/Cursor session.
- Existing regression suites for `progdetector`, `localapi`, `cmd/shellman`, and `projectstate`.
