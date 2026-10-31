# Architecture

```mermaid
flowchart LR
    Chat[Server-owned chat log] --> Parse[Strict log parser]
    Parse --> Auth[Fresh OP authorization]
    Auth --> Plan[Planner interface]
    Plan --> Policy[Validate operation and recheck OP]
    Policy --> Audit[Durable started record]
    Audit --> RCON[Bounded RCON operation]
    RCON --> Verify[Verify result]
    Verify --> Outcome[Durable outcome record]
    Outcome --> Reply[Private chat reply]
```

- `internal/minecraft`: parsing/tailing and fail-closed OP checks. Standard online-mode server logs are a trusted input, not cryptographic identity evidence.
- `internal/bot`: planner interface, policy, serialized execution, verification and journal. The built-in planner recognizes two explicit operations. Tests inject an untrusted planner to exercise enforcement.
- `internal/rcon`: length-bounded protocol client with authentication, deadlines, cancellation and multipart/empty responses.
- `internal/config`: strict JSON configuration; requires loopback RCON and keeps credentials outside files.
- `cmd/serverbot`: CLI, cancellation and private chat delivery.
- `tools/check`, `tools/release`: portable CI entry points.

There is no general-purpose shell or file-write tool exposed to a planner. The planned OpenCode adapter must be independently constrained so its remote session cannot invoke unrestricted host tools. Prompt instructions alone are insufficient.

RCON and audit writes do not form a transaction. Failed execution/verification can leave uncertain state. Started jobs are not retried automatically. Recovery and supervision are required before adding disruptive operations.

GitHub Actions tests Windows/Linux natively; GitLab CI runs the shared checks in a Linux container. Both package Windows/Linux amd64/arm64; arm64 runtime tests are not yet implemented. GitHub is the primary repository, with explicit feature-branch pushes to GitLab using `tools/sync`. Automated live deployment remains a later milestone.
