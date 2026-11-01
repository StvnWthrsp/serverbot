# OpenCode planner

Minecraft Server Agent supports OpenCode **2.0.32** as an optional natural-language planner. The default remains explicit commands. It calls the pinned runtime's stateless `POST /api/experimental/generate` endpoint, never an agent session, Build agent, command runner or shell API. Choose any provider/model supported by this runtime; see the [OpenRouter example](openrouter.md).

The reviewed implementation calls the model without tools and returns text. Go strictly decodes a proposal and independently enforces authorization, operation/argument validation, target binding, audit and RCON verification. A model cannot introduce another administrative operation. Whitelist targets must appear explicitly in the player's request.

## Setup

1. Install `@opencode/cli@2.0.32`. Minecraft Server Agent checks `/api/info` before each generation and rejects other versions until reviewed and tested.
2. Use a dedicated account/configuration directory with only the intended provider/model. Add the deny-all permissions from `examples/opencode/opencode.json`. Keep unrelated plugins and personal sessions out of this service. Provider credentials belong in OpenCode's credential management, not Minecraft Server Agent source/configuration.
3. Start the headless runtime on loopback:

```sh
opencode serve --hostname 127.0.0.1 --port 4096
```

OpenCode prints its generated server password at startup; keep that output private. Supply it through `MSA_OPENCODE_PASSWORD`. `MSA_OPENCODE_USERNAME` defaults to `opencode`. These credentials grant access to OpenCode's HTTP server, so restrict them to the Minecraft Server Agent process and keep the service local. Minecraft Server Agent does not change the service configuration itself.

4. Copy `config.opencode.example.json` to your private `minecraft-server-agent.json`. Set the Minecraft path and your configured `provider_id`/`model_id`. The URL must be a literal loopback HTTP(S) origin with a port. Proxies and redirects are disabled; URL credentials are rejected.
5. Connect your provider as described below, then preview without RCON or Minecraft changes:

```sh
minecraft-server-agent plan -config minecraft-server-agent.json -message "minecraft-server-agent, please whitelist Friend_1"
```

Expected proposal:

```json
{"action":"whitelist_add","target":"Friend_1"}
```

`plan` is a local diagnostic command; it does not authorize a player or execute a proposal. It reserves an inference attempt in the local ledger, using the same [hourly allowance](request-limits.md) as chat and `doctor -check-model`. For chat administration, also supply `MSA_RCON_PASSWORD`, run `minecraft-server-agent doctor`, then `minecraft-server-agent run`. The watcher checks OP status before contacting the model and again before execution. Requests begin with `minecraft-server-agent`, optionally followed by a comma or colon. For guided configuration, start with [setup](setup.md).

## Provider authentication

Set `opencode.provider_id` and `opencode.model_id` in `minecraft-server-agent.json` to your chosen provider and model. Keep credentials out of that file.

For an API key, supply it in the Minecraft Server Agent process environment as `MSA_PROVIDER_API_KEY`, then run:

```sh
minecraft-server-agent connect -config minecraft-server-agent.json
```

To use an existing environment variable instead:

```sh
minecraft-server-agent connect -config minecraft-server-agent.json -key-env OPENROUTER_API_KEY
```

This explicit setup command sends the key to the authenticated loopback OpenCode server, which saves it in its own credential store. It does not contact Minecraft, invoke a model, print the key or write it to Minecraft Server Agent configuration. Remove the variable from the Minecraft Server Agent environment after setup. Protect OpenCode's data directory as credential storage. A saved credential does not confirm that a particular model is available; use `minecraft-server-agent plan` to check that separately.

For OAuth, run `opencode auth login --standalone` under the same dedicated account and configuration/data directories as the headless runtime. Choose your provider and its OAuth method when offered, and follow OpenCode's sign-in prompts. OAuth availability depends on the provider; Minecraft Server Agent reuses the connection managed by OpenCode. Restart the headless runtime after signing in with the separate CLI. Existing OpenCode provider connections can also be reused without running `minecraft-server-agent connect`.

## Proposals and failures

The model returns exactly `action` and `target`, both strings. Status uses `{"action":"status","target":""}`. Unsupported/ambiguous requests should return `{"action":"refuse","target":""}`, which executes nothing. Unknown operations/fields, duplicate fields, code fences, nulls, multiple objects, invalid usernames and unmentioned targets are rejected. Restart, configuration changes, downloads and OP changes remain unsupported.

Each request has one generation attempt and a total deadline. Transport/provider failures, authentication errors and incompatible versions fail closed with no unrestricted fallback. Responses are capped at 64 KiB, proposals at 1 KiB and player text at 512 bytes. Only player text is sent: no Minecraft logs, paths, OP lists, RCON passwords or other server data.

Timeout/cancellation stops the local request and prevents late execution. It cannot guarantee that a remote provider stops computation/billing. The endpoint has no per-call token/cost parameter; the OpenRouter example applies an output-token cap through OpenCode provider configuration. Minecraft Server Agent enforces a durable rolling hourly attempt limit; configure provider-side token/spending limits as well. Token/currency accounting remains roadmap work. Constrained tools do not guarantee correct understanding of player intent; a Windows pilot remains pending.

## Contract testing

Both CI providers run `go run ./tools/contract`. This Linux amd64 helper downloads the pinned npm platform archive, checks its committed SHA-512 digest, extracts only the executable into temporary storage, and runs the actual runtime against a local fake model. Separate config/data and synthetic credentials isolate the test. It checks that provider requests contain no tools and uses neither paid models nor Minecraft.

Other hosts can set `MSA_TEST_OPENCODE` to an installed executable's full path and run:

```sh
go test -race -run TestPinnedRuntimeContract -v ./internal/opencode
```

The ordinary suite tests HTTP failures, cancellation, malicious output, OP revocation and unauthorized requests without OpenCode installed.

Reviewed sources: [generation](https://github.com/anomalyco/opencode/blob/v2.0.32/packages/core/src/generate.ts), [handler](https://github.com/anomalyco/opencode/blob/v2.0.32/packages/server/src/handlers/generate.ts), [protocol](https://github.com/anomalyco/opencode/blob/v2.0.32/packages/protocol/src/groups/generate.ts). Current online documentation may describe newer routes.
