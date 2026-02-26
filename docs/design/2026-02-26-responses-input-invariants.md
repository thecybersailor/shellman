# Responses Input Invariants (2026-02-26)

## Invariants

- Invariant-1: `system` message count must be `<= 1`.
- Invariant-2: if a `system` message exists, it must be at index `0` (first item).
- Invariant-3: every `function_call_output.call_id` must reference a prior `function_call.call_id`.

## Enforcement Points

- `LoopRunner`: validate after round-mode hint injection and before each network request.
- `ResponsesClient.toSDKRequest`: validate before SDK serialization.

## Failure Policy

- Fail-fast locally on invariant violation.
- Never send invalid payloads to provider.
- Error message must include invariant type and input index/call_id for debugging.
