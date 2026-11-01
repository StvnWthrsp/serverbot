# Minecraft Server Agent

Go Minecraft administration assistant. Work on feature branches. GitHub is primary; use `go run ./tools/sync` to push feature branches to GitHub and GitLab. Keep checks portable between both providers. Run `go run ./tools/check` before committing. For planner changes also run `go run ./tools/contract` on Linux amd64.

Never copy credentials, player lists, server worlds, or historical OpenCode sessions into this repository. Do not operate the live Minecraft server as part of development. Tests use isolated fixtures and local fake services.

The agent/planner may propose operations; authorization and operation validation must be enforced by the Go application before any command executes. New mutations need integration tests, verification and an explicit recovery design. Preserve fail-closed authorization and durable audit-before-action behavior.
