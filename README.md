# Minecraft Server Agent

A Go chat bridge that lets trusted Minecraft OPs use a full OpenCode agent on the machine running their server.

Ask it to whitelist friends, restart the server, investigate errors, install compatible mods, or perform other administration. OpenCode keeps its normal system and web tools; Minecraft Server Agent supplies OP authentication, persistent conversations, chat replies, request controls and dispatch auditing.

**Status: experimental.** The full-agent bridge is tested against OpenCode 2.0.32 using real tools and isolated fixtures. A real Minecraft/Windows pilot and service installation remain pending.

## How it works

1. Watches new vanilla/Forge server-thread chat messages beginning with `minecraft-server-agent`.
2. Checks the player's OP entry and `online-mode=true` before queue admission and again before agent dispatch.
3. Records the request and session ownership before forwarding it to that OP's OpenCode conversation in the Minecraft directory.
4. OpenCode carries out the request with its normal tools, including shell, filesystem and web access.
5. Returns the agent's text or questions privately through Minecraft RCON.

For example:

```text
minecraft-server-agent please whitelist Friend_1
minecraft-server-agent restart the server when everyone has logged off
minecraft-server-agent find a compatible version of this mod and install it
minecraft-server-agent /stop
minecraft-server-agent /new
```

Each OP has a separate persistent session keyed by UUID. Follow-up messages and question answers return to the same session. `/stop` uses a separate control queue so it can interrupt busy work; `/new` starts a fresh conversation on the next request. See [agent behavior and configuration](docs/agent.md).

## Build and setup

Install the Go version in `go.mod`, then:

```sh
go build -o bin/minecraft-server-agent ./cmd/minecraft-server-agent
./bin/minecraft-server-agent setup
```

On Windows build with `-o bin/minecraft-server-agent.exe` and use `.\bin\minecraft-server-agent.exe`. The wizard defaults to full-agent mode, guides provider/model authentication and creates `minecraft-server-agent.json` without overwriting an existing file. You can also copy `config.agent.example.json`.

Start OpenCode 2.0.32 on the same machine with system/web permissions, using [the runtime setup](docs/opencode.md). Supply its startup password as `MSA_OPENCODE_PASSWORD` and Minecraft's RCON password as `MSA_RCON_PASSWORD`. Keys stay outside the application configuration; [API-key and OAuth setup](docs/opencode.md#provider-authentication) works with your chosen provider, including [OpenRouter](docs/openrouter.md).

```sh
./bin/minecraft-server-agent doctor
./bin/minecraft-server-agent run
```

Normal setup/doctor make no model calls. `doctor -check-model` performs an optional read-only connectivity probe. Start the watcher before sending chat; it begins at EOF. Stop it with Ctrl+C. See [first-run setup](docs/setup.md).

Record the server's startup/stop commands, Minecraft/mod-loader versions, backup locations and recovery procedure in its own `AGENTS.md`. OpenCode can use those workspace instructions instead of having to rediscover them for every administrator.

## Trust and execution

**OP means trusted machine administrator.** Full-agent sessions receive allow-all OpenCode tool permissions and execute under the OpenCode account's OS privileges. The Minecraft directory is a working directory, not a filesystem sandbox. The bridge does not impose a whitelist/status operation allowlist, inspect every shell command or provide transaction/rollback guarantees. Tool execution, verification and recovery are handled by OpenCode and the server's operating instructions. Tool results and workspace content may be sent to the selected inference provider.

OP checks control who can dispatch requests. They do not protect against a compromised account/mod, prompt injection or a mistaken agent action. The log adapter assumes server-owned logs and standard formatting; it is not a cryptographically authenticated player-event API. Offline-mode/proxy authentication and custom chat decorations are unsupported.

The bridge monitors authorization during active foreground requests and attempts interruption on revocation, cancellation or timeout. On watcher startup/shutdown it interrupts only sessions recorded as its own; requests are never replayed automatically. Interruption cannot undo completed changes or guarantee termination of detached processes. Inspect the OpenCode session and actual server state before retrying uncertain work.

Audit records identify requests, OP UUIDs, sessions and dispatch/outcome states. OpenCode owns the detailed tool transcript. Restrict both data directories with Unix permissions or Windows ACLs. After a crash, stop all bridge instances, inspect state, then remove stale journal/ledger `.lock` directories before restarting. Firewall RCON to local access; the client loopback setting does not bind Minecraft's listener.

## Limits and optional modes

Defaults: 10-second UUID cooldown, eight waiting requests plus one active request, and 60 agent interactions per rolling hour. Question answers bypass the cooldown but still consume an interaction. The interaction ledger survives restarts. One interaction can involve many model calls; use OpenCode agent step/output settings and provider spending limits as well. See [limits and recovery](docs/request-limits.md).

Existing `planner: "commands"` and `planner: "opencode"` profiles remain available for explicit commands and restricted proposals. Full agent execution uses `planner: "agent"`; existing profiles are not silently converted. `plan` always remains a tool-free, read-only diagnostic and never runs an agent.

The local `request -player Admin -message "..."` command trusts the machine owner's supplied identity, checks that name against OP permissions and executes through the selected mode. Do not expose it as a remote API.

## Development and distribution

```sh
go run ./tools/check
go run ./tools/contract
go run ./tools/release -version dev
```

Shared checks enforce formatting, vet, race tests and builds. Both GitHub Actions and GitLab CI run vulnerability scans, real pinned-runtime contracts against fake providers, and four-target Windows/Linux packaging. The agent contract executes shell/web tools against temporary fixtures, bridges native questions and verifies interruption. No live Minecraft server or paid inference is used.

GitHub is [public and primary](https://github.com/StvnWthrsp/minecraft-server-agent); [GitLab](https://gitlab.com/stvnwthrsp/minecraft-server-agent) is the private CI copy. Use `go run ./tools/sync` for explicit feature-branch synchronization. Version tags create draft releases; no release is published or server deployed automatically. See [CI/CD](docs/ci-cd.md), [architecture](docs/architecture.md) and [roadmap](docs/roadmap.md).
