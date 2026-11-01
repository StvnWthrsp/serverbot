# Security

Minecraft Server Agent is experimental; there is no supported stable release yet.

Report vulnerabilities using GitHub private vulnerability reporting or contact the repository owner privately. Never post credentials or private logs in public issues.

Full-agent mode intentionally gives authenticated OP administrators access to OpenCode's normal system and web tools with allow-all session permissions. They must be trusted with the runtime account's OS privileges. The configured Minecraft directory is a working directory, not a sandbox. The Go bridge does not validate each tool command or limit agent administration to a fixed operation list.

Authorization, session ownership, dispatch auditing and request controls do not prevent prompt injection, agent mistakes, compromised mods/accounts or actions taken independently in OpenCode. Runtime tool output and local file contents may reach the selected inference provider. Use the OS account, OpenCode configuration and provider settings appropriate for your intended access.

The log adapter trusts server-owned logs and standard formatting. It requires online-mode=true; authenticated UUID events/proxy authentication remain future work. A local caller of `request` can supply an OP name and must already be trusted as the machine owner; do not expose that command remotely.

Keep OpenCode's authenticated HTTP endpoint on loopback and firewall RCON to local access. A loopback client destination does not bind Minecraft's listener. Restrict both applications' data/transcript directories with Unix permissions or Windows ACLs.

Interruption, revocation detection and deadlines are best effort. Completed changes and detached processes can survive cancellation or a bridge crash. Inspect OpenCode/server state before retrying uncertain work and maintain normal backups/recovery instructions. Requests are not automatically replayed.

Optional explicit-command/read-only-planner modes keep their separate Go operation/target validation. They are not the full-agent product's tool boundary.
