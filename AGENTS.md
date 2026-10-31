# ServerBot

Go Minecraft administration assistant. Work on feature branches. Keep checks portable between GitHub Actions and future GitLab CI. Run `go run ./tools/check` before committing.

Never copy credentials, player lists, server worlds, or historical OpenCode sessions into this repository. Do not operate the live Minecraft server as part of development. Tests use isolated fixtures and local fake services.

The agent/planner may propose operations; authorization and operation validation must be enforced by the Go application before any command executes. New mutations need integration tests, verification and an explicit recovery design. Preserve fail-closed authorization and durable audit-before-action behavior.
