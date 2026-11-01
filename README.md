# ServerBot

A Go Minecraft administration assistant being rebuilt from a personal PowerShell prototype. The goal is to let trusted players handle routine administration through chat, with bounded operations, verification and an audit trail.

**Status: experimental.** Supports explicit commands and optional natural-language whitelist/status proposals through a pinned, tool-free OpenCode integration. Supervised restarts, automatic recovery and service installation remain planned. No live Minecraft server or paid model is required for development or CI.

## What works

- Reads new vanilla/Forge server-thread chat lines, starting at EOF. Trigger: `serverbot` at the beginning of the message, case insensitive.
- Checks `ops.json` for every request and immediately before execution. Fails closed on missing/malformed permissions and requires `online-mode=true`.
- Supports `serverbot status`, `serverbot whitelist Friend_1`, and `serverbot whitelist add Friend_1`.
- With the optional OpenCode planner, understands requests such as `serverbot, please whitelist Friend_1`. Go validates proposals and requires targets to be explicitly named.
- Connect your chosen provider through OpenCode using an API key or its supported OAuth sign-in; see [provider setup](docs/opencode.md#provider-authentication) and the [OpenRouter example](docs/openrouter.md).
- Executes validated operations through local RCON. Whitelist additions are verified using `whitelist list` before success is reported.
- Replies privately to the requesting player with JSON-encoded `tellraw` messages.
- Records jobs to a synced JSONL journal before execution. Previously started jobs are not automatically replayed after restart.
- Runs Go checks on Windows/Linux in GitHub Actions. Packages Windows/Linux amd64/arm64 binaries with checksums.

## Build and configure

Install the Go version declared in `go.mod` or later, then:

```sh
go build -o bin/serverbot ./cmd/serverbot
```

On Windows use `-o bin/serverbot.exe`. Copy `config.example.json` to `serverbot.json` and set the server directory. Relative paths resolve against the configuration file's directory. Configure Minecraft RCON yourself, keep it firewalled from remote clients, and supply its password in the `SERVERBOT_RCON_PASSWORD` process environment variable. Never put it in the configuration file or repository.

PowerShell:

```powershell
# Supply SERVERBOT_RCON_PASSWORD through your process/service secret configuration.
.\bin\serverbot.exe doctor -config serverbot.json
.\bin\serverbot.exe run -config serverbot.json
```

Linux:

```sh
# Supply SERVERBOT_RCON_PASSWORD through your process/service secret configuration.
./bin/serverbot doctor -config serverbot.json
./bin/serverbot run -config serverbot.json
```

`doctor` checks required files and RCON connectivity; per-player authorization is checked when handling a request. It does not modify Minecraft configuration. Start the watcher before sending chat commands. Stop it with Ctrl+C.

For a local diagnostic request without sending chat:

```sh
./bin/serverbot request -config serverbot.json -player Admin -message "serverbot status"
```

This diagnostic command trusts the local caller's supplied name. It is for the machine owner and must not be exposed as an unauthenticated remote API.

## Trust and recovery

The log adapter assumes server-owned, trustworthy logs and standard formatting. Display-name parsing is not an authenticated player-event API. Offline-mode/proxy authentication, custom chat formats and mod-provided decorations are unsupported. A future server integration will provide authenticated UUID events. The watcher is not a security boundary against compromised mods or local accounts.

ServerBot never executes shell commands from chat or grants planners arbitrary machine access. The optional OpenCode adapter uses the pinned stateless, tool-free generation API; proposals pass the same Go authorization and validation as explicit commands. The legacy unrestricted Build integration is not included. See [OpenCode setup](docs/opencode.md) for the version pin, configuration, limits and read-only `serverbot plan` preview.

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
