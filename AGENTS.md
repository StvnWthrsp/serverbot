# Minecraft Server Agent

Go Minecraft administration assistant. Work on feature branches. GitHub is primary; use `go run ./tools/sync` to push feature branches to GitHub and GitLab. Keep checks portable between both providers. Run `go run ./tools/check` before committing. For planner changes also run `go run ./tools/contract` on Linux amd64.

Never copy credentials, player lists, server worlds, or historical OpenCode sessions into this repository. Do not operate the live Minecraft server as part of development. Tests use isolated fixtures and local fake services.

The product is an OP-authenticated chat bridge to OpenCode's full agent, including system and web tools, as explicitly requested by the owner. Preserve OP authorization before dispatch, request limits, session ownership and durable audit-before-dispatch. Do not replace this with a whitelist/status-only planner or restrict the agent's tool set. The optional read-only planner/explicit command paths retain Go operation validation. Test agent tool execution, cancellation and recovery using isolated fixtures; never use the live Minecraft server or paid inference for CI.
