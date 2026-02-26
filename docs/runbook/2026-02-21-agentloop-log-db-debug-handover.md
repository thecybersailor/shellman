# Agentloop Log/DB Debug Handover

## Responses Input Invariant Debug

- Invariant-1: `system` message count must be `<= 1`.
- Invariant-2: if `system` exists, it must be first.
- Invariant-3: `function_call_output.call_id` must reference a previous `function_call`.
- Policy: fail-fast locally; do not send invalid payload to provider.

## Triage Steps

1. Check LoopRunner error: `responses input invariant failed ...`.
2. Check invariant details (`index`, `call_id`, and invariant text) in wrapped error.
3. Confirm whether round-mode hint insertion changed message order.
4. Verify serialized request path also rejects invalid input in `toSDKRequest`.
