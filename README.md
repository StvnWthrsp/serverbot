# Minecraft Server Agent

A Go Minecraft administration assistant being rebuilt from a personal PowerShell prototype. The goal is to let trusted players handle routine administration through chat, with bounded operations, verification and an audit trail.

**Status: experimental.** Supports explicit commands and optional natural-language whitelist/status proposals through a pinned, tool-free OpenCode integration. Supervised restarts, automatic recovery and service installation remain planned. No live Minecraft server or paid model is required for development or CI.

## What works

- Reads new vanilla/Forge server-thread chat lines, starting at EOF. Trigger: `minecraft-server-agent` at the beginning of the message, case insensitive.
- Checks `ops.json` for every request and immediately before execution. Fails closed on missing/malformed permissions and requires `online-mode=true`.
- Supports `minecraft-server-agent status`, `minecraft-server-agent whitelist Friend_1`, and `minecraft-server-agent whitelist add Friend_1`.
- With the optional OpenCode planner, understands requests such as `minecraft-server-agent, please whitelist Friend_1`. Go validates proposals and requires targets to be explicitly named.
- Connect your chosen provider through OpenCode using an API key or its supported OAuth sign-in; see [provider setup](docs/opencode.md#provider-authentication) and the [OpenRouter example](docs/openrouter.md).
- Executes validated operations through local RCON. Whitelist additions are verified using `whitelist list` before success is reported.
- Replies privately to the requesting player with JSON-encoded `tellraw` messages.
- Records jobs to a synced JSONL journal before execution. Previously started jobs are not automatically replayed after restart.
- Guides first-run configuration with `setup` and reports actionable connection diagnostics with `doctor`.
- Enforces durable per-player cooldowns, a bounded request queue and an hourly inference-attempt allowance, including previews and failed attempts.
- Runs Go checks on Windows/Linux in GitHub Actions. Packages Windows/Linux amd64/arm64 binaries with checksums.

## Build and configure

Install the Go version declared in `go.mod` or later, then:

```sh
go build -o bin/minecraft-server-agent ./cmd/minecraft-server-agent
```

On Windows use `-o bin/minecraft-server-agent.exe`. Run the setup wizard from the extracted release directory (or use `./bin/minecraft-server-agent` after building):

```sh
minecraft-server-agent setup
```

It guides Minecraft/RCON paths, optional OpenCode provider/model authentication, request limits and connection checks. It creates `minecraft-server-agent.json` and refuses to overwrite an existing file. See [first-run setup](docs/setup.md). Alternatively, copy and edit either example configuration. Relative paths resolve against the configuration file's directory. Configure Minecraft RCON yourself, keep it firewalled from remote clients, and supply its password in the `MSA_RCON_PASSWORD` process environment variable. Never put it in the configuration file or repository.

PowerShell:

```powershell
# Supply MSA_RCON_PASSWORD through your process/service secret configuration.
.\bin\minecraft-server-agent.exe doctor -config minecraft-server-agent.json
.\bin\minecraft-server-agent.exe run -config minecraft-server-agent.json
```

Linux:

```sh
# Supply MSA_RCON_PASSWORD through your process/service secret configuration.
./bin/minecraft-server-agent doctor -config minecraft-server-agent.json
./bin/minecraft-server-agent run -config minecraft-server-agent.json
```

`doctor` checks required files, online-mode/OP settings, RCON and the optional OpenCode runtime independently, reporting how to fix each failure. Use `doctor -check-model` for an optional budgeted inference check. It does not modify Minecraft configuration. Start the watcher before sending chat commands. Stop it with Ctrl+C.

For a local diagnostic request without sending chat:

```sh
./bin/minecraft-server-agent request -config minecraft-server-agent.json -player Admin -message "minecraft-server-agent status"
```

This diagnostic command trusts the local caller's supplied name. It is for the machine owner and must not be exposed as an unauthenticated remote API.

## Trust and recovery

The log adapter assumes server-owned, trustworthy logs and standard formatting. Display-name parsing is not an authenticated player-event API. Offline-mode/proxy authentication, custom chat formats and mod-provided decorations are unsupported. A future server integration will provide authenticated UUID events. The watcher is not a security boundary against compromised mods or local accounts.

Minecraft Server Agent never executes shell commands from chat or grants planners arbitrary machine access. The optional OpenCode adapter uses the pinned stateless, tool-free generation API; proposals pass the same Go authorization and validation as explicit commands. The legacy unrestricted Build integration is not included. See [OpenCode setup](docs/opencode.md) for the version pin, configuration and `minecraft-server-agent plan` preview, which executes no Minecraft operations. Inference attempts are recorded locally to enforce the budget.

Defaults are a 10-second per-player cooldown, eight waiting requests plus one active request, and 60 inference attempts per rolling hour. One player can have only one queued/active request. Queued time counts toward the request deadline; expired work is discarded before planning. Full-queue and duplicate-pending requests are logged and dropped without extra model calls or RCON replies. See [request limits](docs/request-limits.md) for configuration, persistence and recovery. Account-level token/spending limits remain the provider's responsibility.

Audit files contain player names/UUIDs and operation metadata. Restrict the data directory with Windows ACLs or Unix permissions. One process may own a journal; a lock directory prevents duplicate ownership. After a crash, stop all instances, inspect journal/server state, and remove `<audit_file>.lock` before restarting. A `started` or `failed` job may already have changed the whitelist; inspect actual state before submitting another request. Corrupt journals fail closed and need manual recovery. This is not a transactional database or an exactly-once execution guarantee.

The watcher handles observed file replacement/truncation and buffers partial lines. Truncate-and-regrow races between polls and arbitrary log rewrites are not reliably detectable. Operation failures appear in process logs. An operation's success and failed chat delivery are separate outcomes. Whitelist verification currently assumes the standard English RCON response.

## Development and CI/CD

```sh
go run ./tools/check
go run ./tools/release -version dev
go run ./tools/contract
```

The check command enforces formatting, runs `go vet`, tests with race detection and coverage, and builds all packages. Race tests require a C compiler, included on GitHub-hosted Windows/Linux runners. Tests use temporary server directories, simulated planners and fake RCON connections. CI also runs pinned `govulncheck`, a real OpenCode contract test with a local fake model, and packaging. The contract helper installs a checksum-pinned runtime on Linux amd64; other hosts can run the opt-in test with an installed executable. Coverage files are uploaded as artifacts.

Tags such as `v0.1.0` rerun Windows/Linux checks, scan for known Go vulnerabilities, package binaries and create a **draft** GitHub release. They do not deploy to Minecraft. No release has been published yet.

GitHub Actions and GitLab CI use the same Go check/packaging commands. GitHub is the primary repository and runs native Windows/Linux tests; GitLab runs Linux verification and produces four-target package artifacts. Use `go run ./tools/sync` to explicitly push a feature branch to both providers. See [CI/CD and synchronization](docs/ci-cd.md), [the roadmap](docs/roadmap.md) and [architecture](docs/architecture.md).
