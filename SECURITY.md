# Security

ServerBot is experimental. Only current development is maintained; there is no supported stable release yet.

Report vulnerabilities through GitHub private vulnerability reporting when enabled, or contact the repository owner privately to arrange a report. Never put credentials or private logs in public issues. Private reporting must be enabled before public launch.

Use a dedicated local account with access to the intended server and audit directory. Firewall RCON to trusted local access. A loopback address in ServerBot configuration controls the client destination, not the Minecraft listener's bind address. Rotate exposed legacy credentials before release deployment.

The log adapter trusts local server logs and standard formatting. Authenticated UUID events, stronger service isolation and recovery for disruptive operations remain release milestones. See README.md for supported scope and limitations.
