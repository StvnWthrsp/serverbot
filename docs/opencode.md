# OpenCode planner

ServerBot supports OpenCode **2.0.32** as an optional natural-language planner. The default remains explicit commands. It calls the pinned runtime's stateless `POST /api/experimental/generate` endpoint, never an agent session, Build agent, command runner or shell API.

The reviewed implementation calls the model without tools and returns text. Go strictly decodes a proposal and independently enforces authorization, operation/argument validation, target binding, audit and RCON verification. A model cannot introduce another administrative operation. Whitelist targets must appear explicitly in the player's request.

## Setup

1. Install `@opencode/cli@2.0.32`. ServerBot checks `/api/info` before each generation and rejects other versions until reviewed and tested.
2. Use a dedicated account/configuration directory with only the intended provider/model. Add the deny-all permissions from `examples/opencode/opencode.json`. Keep unrelated plugins and personal sessions out of this service. Provider credentials belong in OpenCode's credential management, not ServerBot source/configuration.
3. Start the headless runtime on loopback:

```sh
opencode serve --hostname 127.0.0.1 --port 4096
```

OpenCode prints its generated server password at startup; keep that output private. Supply it through `SERVERBOT_OPENCODE_PASSWORD`. `SERVERBOT_OPENCODE_USERNAME` defaults to `opencode`. These credentials grant access to OpenCode's HTTP server, so restrict them to the ServerBot process and keep the service local. ServerBot does not change the service configuration itself.

4. Copy `config.opencode.example.json` to your private `serverbot.json`. Set the Minecraft path and your configured `provider_id`/`model_id`. The URL must be a literal loopback HTTP(S) origin with a port. Proxies and redirects are disabled; URL credentials are rejected.
5. Preview without RCON or Minecraft changes:

```sh
serverbot plan -config serverbot.json -message "serverbot, please whitelist Friend_1"
```

Expected proposal:

```json
{"action":"whitelist_add","target":"Friend_1"}
```

`plan` is a local diagnostic command; it does not authorize a player or execute a proposal. For chat administration, also supply `SERVERBOT_RCON_PASSWORD`, run `serverbot doctor`, then `serverbot run`. The watcher checks OP status before contacting the model and again before execution. Requests begin with `serverbot`, optionally followed by a comma or colon.

## Proposals and failures

The model returns exactly `action` and `target`, both strings. Status uses `{"action":"status","target":""}`. Unsupported/ambiguous requests should return `{"action":"refuse","target":""}`, which executes nothing. Unknown operations/fields, duplicate fields, code fences, nulls, multiple objects, invalid usernames and unmentioned targets are rejected. Restart, configuration changes, downloads and OP changes remain unsupported.

Each request has one generation attempt and a total deadline. Transport/provider failures, authentication errors and incompatible versions fail closed with no unrestricted fallback. Responses are capped at 64 KiB, proposals at 1 KiB and player text at 512 bytes. Only player text is sent: no Minecraft logs, paths, OP lists, RCON passwords or other server data.

Timeout/cancellation stops the local request and prevents late execution. It cannot guarantee that a remote provider stops computation/billing. The endpoint has no per-call token/cost parameter; configure provider-side limits and budgets. Global rate/cost budgeting remains roadmap work. Constrained tools do not guarantee correct understanding of player intent; a real-provider quality evaluation and Windows pilot remain separate validation steps.

## Contract testing

Both CI providers run `go run ./tools/contract`. This Linux amd64 helper downloads the pinned npm platform archive, checks its committed SHA-512 digest, extracts only the executable into temporary storage, and runs the actual runtime against a local fake model. Separate config/data and synthetic credentials isolate the test. It checks that provider requests contain no tools and uses neither paid models nor Minecraft.

Other hosts can set `SERVERBOT_TEST_OPENCODE` to an installed executable's full path and run:

```sh
go test -race -run TestPinnedRuntimeContract -v ./internal/opencode
```

The ordinary suite tests HTTP failures, cancellation, malicious output, OP revocation and unauthorized requests without OpenCode installed.

Reviewed sources: [generation](https://github.com/anomalyco/opencode/blob/v2.0.32/packages/core/src/generate.ts), [handler](https://github.com/anomalyco/opencode/blob/v2.0.32/packages/server/src/handlers/generate.ts), [protocol](https://github.com/anomalyco/opencode/blob/v2.0.32/packages/protocol/src/groups/generate.ts). Current online documentation may describe newer routes.
