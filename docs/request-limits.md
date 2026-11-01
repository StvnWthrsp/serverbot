# Request and inference limits

Both example configurations and `setup` include:

```json
"limits": {
  "player_cooldown_seconds": 10,
  "queue_capacity": 8,
  "inference_calls_per_hour": 60
}
```

Omitting the whole object uses these defaults. Explicit values must be positive: cooldown 1–3600 seconds, queue capacity 1–100, inference allowance 1–10000. Invalid or incomplete limits fail configuration validation. Changes require restarting the application.

## Chat admission and cooldown

Only freshly authorized OP players can enter the queue. It holds at most `queue_capacity` waiting requests plus one active request, with only one pending/active request per player UUID. Overflow, duplicate-pending and unauthorized requests are dropped and logged; the log reader never waits for inference or RCON. No extra RCON response is sent for those rejected submissions.

Requests have a total deadline from admission, including queue time. Expired queued requests are discarded without planning/execution. Shutdown cancels active work and discards waiting work. The queue is in memory and is not replayed after a restart.

The engine checks authorization again when processing and immediately before execution. It records each accepted request durably in the execution journal before planning. The cooldown is keyed by UUID, so changing name casing does not bypass it. Refused/failed planning also consumes the cooldown. Reopening the same journal preserves it; a duplicate request ID is not replayed. Requests blocked by the cooldown make no inference call and perform no administration.

## Durable inference allowance

All OpenCode planning paths share `<audit_file>.inference`: chat, local `request`, `plan`, and `doctor -check-model`. Each attempt reserves a slot before contacting OpenCode. Reservations are counted over the preceding rolling hour, including refusals, transport failures, cancellation after reservation, and incompatible runtime responses. There are no automatic retries. Explicit command planning and provider credential connection do not consume inference slots.

The ledger uses exclusive file ownership and synced writes. Concurrent processes cannot exceed a shared allowance; a busy/corrupt/unwritable ledger fails closed. Recreating a planner or restarting the application does not reset reservations. Future timestamps count conservatively if the clock moves backward. Keep the host clock accurate.

All processes must use the same audit path/ledger to share this allowance. Changing that path, deleting the ledger, or using a different copy creates a new allowance. Do not rotate/delete current-hour reservations to reclaim capacity. After a crash, stop all instances and inspect both journal/ledger before removing stale `.lock` directories. An inference reservation can exist even if no provider call completed; treat it as consumed until it expires.

This limits attempted inference requests, not tokens or currency. The pinned API exposes no per-call token budget or reliable usage/cost result. Set model output limits in OpenCode provider configuration and account spending limits with the provider. Calls made outside Minecraft Server Agent are outside its allowance. A remote provider may continue computing/billing after local cancellation.
