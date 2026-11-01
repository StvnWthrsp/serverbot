# OpenCode runtime

Minecraft Server Agent supports OpenCode **2.0.32**. The main `agent` mode uses its persistent session API and full tool loop. The optional `opencode` planner and local `plan` diagnostic use the separate stateless generation endpoint.

## Setup

1. Install `@opencode/cli@2.0.32`. Runtime compatibility is pinned and tested; other versions are rejected until reviewed.
2. Configure the runtime with `examples/opencode/opencode.json` (or [OpenRouter](openrouter.md)). These examples allow tools for trusted administrators. Preserve any agent, plugin or MCP configuration you want to expose. Run OpenCode under the account whose server/system access you intend OPs to have.
3. Start it on the same machine, listening on loopback:

```sh
opencode serve --hostname 127.0.0.1 --port 4096
```

Keep the generated startup password private. Supply it to the bridge as `MSA_OPENCODE_PASSWORD`; `MSA_OPENCODE_USERNAME` defaults to `opencode`. These are local HTTP credentials, separate from your provider key/OAuth tokens. The bridge's client uses a literal loopback URL, disables proxies/redirects and rejects URL credentials. This does not limit the agent's own system/web tools.

4. Run `minecraft-server-agent setup` or copy `config.agent.example.json`, selecting your provider/model and Minecraft path.
5. Connect your provider below, then run `doctor` and `run`. See [full-agent behavior](agent.md).

## Provider authentication

Set `opencode.provider_id` and `opencode.model_id` to your chosen provider and model. Choose a model supporting tool calls for agent mode. Keep credentials out of the JSON configuration.

Supply a key through the bridge's environment:

```sh
minecraft-server-agent connect -config minecraft-server-agent.json
```

This reads `MSA_PROVIDER_API_KEY`. To select another existing variable:

```sh
minecraft-server-agent connect -config minecraft-server-agent.json -key-env OPENROUTER_API_KEY
```

The explicit setup command sends the key to authenticated loopback OpenCode, which saves it in its credential store. It makes no Minecraft/model calls and never prints the key or writes it into the bridge configuration. Remove the key variable from the bridge environment after setup and protect OpenCode's data directory.

For OAuth, use `opencode auth login --standalone` under the same account and configuration/data directories as the headless runtime. Select your provider's OAuth method when offered and follow its sign-in flow, then restart the headless runtime. Existing provider connections can be reused without `connect`. OAuth availability depends on the provider.

## Read-only connectivity and proposals

Normal `doctor` checks runtime connectivity/version without making inference calls. `doctor -check-model` performs one stateless status probe with no tools and no Minecraft administration.

```sh
minecraft-server-agent plan -message "minecraft-server-agent please whitelist Friend_1"
```

`plan` always returns a restricted JSON proposal, even with an agent profile. It never starts a tool-using session. To actually execute an agent request, use authenticated chat or the machine-owner diagnostic `request -player Admin -message "..."`.

Existing `planner: "opencode"` profiles still use the restricted whitelist/status proposal path, with Go operation/target validation. Set `planner: "agent"` to use the full tool loop; old configurations are not silently escalated. `config.opencode.example.json` documents the optional proposal profile, while `config.agent.example.json` is the primary agent profile.

The diagnostic/planner path uses `limits.inference_calls_per_hour`; full-agent interactions use `limits.agent_runs_per_hour` and may involve multiple model calls. See [limits](request-limits.md).

## Contract testing

Both CI providers run `go run ./tools/contract`. It verifies a committed SHA-512 for the pinned npm executable and runs isolated real-runtime tests with a local fake provider. The agent test executes a shell fixture, fetches a local web fixture, bridges a native question/answer, reuses the conversation and interrupts a waiting question. The stateless test separately verifies no tools are exposed by the read-only diagnostic.

Tests use isolated HOME/config/data directories and synthetic credentials; no paid model, real player data or live Minecraft server is used. Other hosts can set `MSA_TEST_OPENCODE` to an installed executable and run:

```sh
go test -race -run 'TestPinnedRuntime.*Contract' -v ./internal/opencode
```

Reviewed interfaces: [sessions](https://github.com/anomalyco/opencode/blob/v2.0.32/packages/protocol/src/groups/session.ts), [messages](https://github.com/anomalyco/opencode/blob/v2.0.32/packages/protocol/src/groups/message.ts), [forms](https://github.com/anomalyco/opencode/blob/v2.0.32/packages/schema/src/form.ts), [stateless generation](https://github.com/anomalyco/opencode/blob/v2.0.32/packages/core/src/generate.ts).
