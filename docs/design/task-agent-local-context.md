# Task Agent Local Context Design

## Scope

This document defines the task-agent context contract used by localapi and agent loop.

## Source Of Truth

- Local source of truth is `task_messages` in SQLite.
- Every turn rebuilds prompt context from local timeline instead of relying on provider-side session memory.
- Provider `store` can still be enabled, but it is not required for cross-turn continuity.

## Prompt Composition

- `USER_INPUT_EVENT` and `TTY_OUTPUT_EVENT` prompts both include:
  - `conversation_history`
  - `context_json`
- `conversation_history` comes from local timeline (`ListTaskMessages(task_id, limit)`).
- Assistant structured message content is normalized before it is injected to history.

## Overflow Strategy

- History builder uses deterministic bounds:
  - `MaxMessages`
  - `MaxChars`
- If overflow happens:
  - Keep recent window
  - Add `history_summary` with dropped/included counts

## Observability

- Context metrics are propagated in trigger metadata and audit logs:
  - `history_total`
  - `history_included`
  - `history_dropped`
  - `history_chars`

## Debug Checklist

1. Check `task_messages` rows for target task (order and content).
2. Check `task.message.send.started` / `task.message.send.agentloop.invoke` audit entries for history metrics.
3. Confirm prompt payload contains `conversation_history`.
4. If model reply still appears stateless, inspect API/provider errors first (for example `invalid_api_key`) before changing context code.
