# Architecture

```mermaid
flowchart LR
    Chat[Server-owned chat log] --> Parse[Strict log parser]
    Parse --> Queue[Fresh OP admission and bounded queue]
    Queue --> Auth[Fresh OP authorization and durable cooldown]
    Auth --> Budget[Durable inference reservation when using OpenCode]
    Budget --> Plan[Planner interface]
    Plan --> Policy[Validate operation and recheck OP]
    Policy --> Audit[Durable started record]
    Audit --> RCON[Bounded RCON operation]
    RCON --> Verify[Verify result]
    Verify --> Outcome[Durable outcome record]
    Outcome --> Reply[Private chat reply]
```

- `internal/minecraft`: parsing/tailing and fail-closed OP checks. Standard online-mode server logs are a trusted input, not cryptographic identity evidence.
- `internal/bot`: bounded queue, durable cooldown/inference limits, planner interface, policy, serialized execution, verification and journal. Validates proposals independently and binds whitelist targets to the player's request. The built-in planner recognizes two explicit operations. A separate inference ledger counts all planning attempts across chat and local diagnostics.
- `internal/opencode`: optional tool-free stateless JSON proposal adapter, pinned to OpenCode 2.0.32. No chat logs/server configuration or credentials are sent to the model. Tests exercise malicious output, HTTP failure/cancellation and OP revocation through the full engine.
- `internal/rcon`: length-bounded protocol client with authentication, deadlines, cancellation and multipart/empty responses.
- `internal/config`: strict JSON configuration; requires loopback RCON and keeps credentials outside files.
- `cmd/minecraft-server-agent`: guided setup, independent diagnostics, CLI, cancellation and private chat delivery.
- `tools/check`, `tools/release`: portable CI entry points.

There is no general-purpose shell or file-write tool exposed to a planner. The OpenCode adapter uses a reviewed stateless model call instead of a remote agent session; its HTTP client cannot redirect or use environment proxies. Both CI providers verify the pinned real runtime sends no tools to a local fake model. Prompt instructions alone are insufficient, so Go still enforces authorization, plan validation and verification.

RCON and audit writes do not form a transaction. Failed execution/verification can leave uncertain state. Started jobs are not retried automatically. Recovery and supervision are required before adding disruptive operations.

GitHub Actions tests Windows/Linux natively; GitLab CI runs the shared checks in a Linux container. Both package Windows/Linux amd64/arm64; arm64 runtime tests are not yet implemented. GitHub is the primary repository, with explicit feature-branch pushes to GitLab using `tools/sync`. Automated live deployment remains a later milestone.
