# Request and inference limits

The agent profile and `setup` use these defaults:

```json
"limits": {
  "player_cooldown_seconds": 10,
  "queue_capacity": 8,
  "agent_runs_per_hour": 60,
  "inference_calls_per_hour": 60
}
```

Omitting the whole object uses these defaults. Cooldown must be 1–3600 seconds, queue capacity 1–100, and each hourly allowance 1–10000. The new agent allowance defaults to 60 when omitted/zero; invalid other fields fail configuration validation. Changes require restarting the application. The setup wizard asks for the allowance appropriate to the selected mode.

## Chat admission and cooldown

Only freshly authorized OP players can enter the queue. It holds at most `queue_capacity` waiting requests plus one active request, with only one pending/active request per player UUID. Overflow, duplicate-pending and unauthorized requests are dropped and logged; the log reader never waits for inference or RCON. No extra RCON response is sent for those rejected submissions.

Requests have a total deadline from admission, including queue time. Expired queued requests are discarded without planning/execution. Shutdown cancels active work and discards waiting work. The queue is in memory and is not replayed after a restart.

The engine checks authorization again when processing and before dispatch. It records each accepted request durably before planning/agent dispatch. The cooldown is keyed by UUID, so changing name casing does not bypass it. Failed requests also consume the cooldown. Answers to an agent's pending question bypass the cooldown but still consume an agent interaction. Reopening the same journal preserves it; duplicate request IDs are not replayed. Requests blocked by the cooldown make no inference/agent call.

Agent mode has a separate authenticated control queue for `/stop` (one waiting control). It can interrupt busy or question-waiting sessions without consuming an agent interaction. `/new` resets the conversation through the ordinary request path without making a model call.

## Agent interactions

`limits.agent_runs_per_hour` counts dispatched administrator interactions in `<audit_file>.agent-runs`, shared by chat and local `request`. Native question answers count as interactions. Reservations are synced before dispatch, survive restarts, and count failures/cancellation conservatively. Busy/corrupt/unwritable ledgers fail closed. Preserve this path/history when updating configuration; a different/deleted ledger creates a new allowance.

One interaction can contain multiple model calls, tools and native retries. This is not a per-model-call or currency cap. Configure OpenCode's agent step limits and provider token/spending budgets separately. The included runtime examples use an adjustable 32-step Build limit; owners may change it. The bridge's request deadline is a best-effort interruption boundary, not a transaction or guarantee that detached processes stop.

## Durable inference allowance

The read-only proposal planner, `plan`, and `doctor -check-model` share `<audit_file>.inference` and `limits.inference_calls_per_hour`. Each attempt reserves a slot before contacting OpenCode. Reservations are counted over the preceding rolling hour, including refusals, transport failures, cancellation after reservation, and incompatible runtime responses. This separate path has no automatic retries. Full-agent interactions use the agent ledger instead; explicit command planning and provider credential connection consume neither allowance.

The ledger uses exclusive file ownership and synced writes. Concurrent processes cannot exceed a shared allowance; a busy/corrupt/unwritable ledger fails closed. Recreating a planner or restarting the application does not reset reservations. Future timestamps count conservatively if the clock moves backward. Keep the host clock accurate.

All processes must use the same audit path/ledger to share this allowance. Changing that path, deleting the ledger, or using a different copy creates a new allowance. Do not rotate/delete current-hour reservations to reclaim capacity. After a crash, stop all instances and inspect both journal/ledger before removing stale `.lock` directories. An inference reservation can exist even if no provider call completed; treat it as consumed until it expires.

The stateless diagnostic endpoint has no per-call token budget/usage result. OpenCode owns agent-loop usage/transcripts. Set model output limits in OpenCode provider configuration and account spending limits with the provider. Calls made outside Minecraft Server Agent are outside its allowances. A remote provider may continue computing/billing after local cancellation.
