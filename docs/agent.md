# Full OpenCode agent

Full-agent mode is the main Minecraft Server Agent workflow. OP chat messages go to persistent OpenCode sessions with the normal Build agent and all tool permissions. There is no Go operation allowlist for this mode: restarts, mod installation, filesystem work, troubleshooting and web research use OpenCode's tools.

## Configuration

Use `setup` (default mode `agent`) or copy `config.agent.example.json`. The existing configuration field is named `planner`; set it to `agent`:

```json
{
  "planner": "agent",
  "opencode": {
    "url": "http://127.0.0.1:4096",
    "provider_id": "your-configured-provider",
    "model_id": "your-configured-model",
    "agent_id": "build"
  }
}
```

Merge these fields into a complete configuration with Minecraft paths/RCON, audit and limits. `agent_id` defaults to `build`; you may select another configured OpenCode agent. New sessions use `server_dir` as their working directory and receive allow-all permissions. Configure OpenCode's own agent, plugins, MCP servers, model and provider as usual; the bridge does not filter its tool inventory.

The agent profile/wizard uses a 900-second deadline, including queued time. Configuration supports 1–3600 seconds. Increase it for long operations if needed. Returning a native question releases the request worker; answering it starts another interaction/deadline. Existing sessions retain their working directory; after changing the Minecraft path, use `/new` to create a session at the new location.

Use `examples/opencode/opencode.json` or `openrouter.json` for a full-tool runtime configuration. These examples allow tools and set an adjustable 32-step Build agent limit. They do not require a two-field response schema.

## Conversations and questions

Every authorized OP UUID owns a separate session. Names changing case do not create another conversation. Ownership/session IDs are recorded durably before the first prompt, then reused for follow-ups and across restarts.

```text
minecraft-server-agent install a compatible version of the mod we discussed
minecraft-server-agent 1
minecraft-server-agent /stop
minecraft-server-agent /new
```

A native OpenCode question is returned privately to its owner. For one question, reply with an option number or free text; use comma-separated choices for a multiselect question. Multiple fields accept a JSON object, for example `minecraft-server-agent {"q0":"Yes","q1":["Option"]}`. Complex external forms can be answered in OpenCode's own interface. Other players cannot answer or stop this session through the bridge.

`/stop` is OP-authenticated and bypasses the busy request queue. It cancels the current bridge request, asks OpenCode to interrupt that OP's session, waits for its foreground loop to settle and cancels undelivered inbox inputs. It does not consume an agent interaction. `/new` interrupts and detaches the old session; the next ordinary request creates a new one. Stop a busy request before asking for a new conversation. The bridge refuses new prompts while a session has unknown active work or undelivered inputs.

Responses are sent as bounded JSON-encoded private RCON messages. Long replies are truncated with a notice; inspect the OpenCode session for the full transcript. Server restarts may temporarily prevent chat delivery even when the agent work completed. Delivery failure is not treated as proof that administration failed.

## Server operating context

Keep server-specific instructions in the Minecraft directory's `AGENTS.md`: Java/Minecraft/loader versions, how to save/stop/start the server, service or launcher commands, mod compatibility requirements, backup locations and restore steps. Do not put credentials in committed instructions. Missing details can be clarified in chat using the same conversation.

The runtime has its account's normal access to the machine and web, including outside `server_dir`. OPs must be trusted with that access. Credentials remain managed by OpenCode; normal tool execution may read local files and send tool results to your inference provider. Request auditing and OP checks are not a sandbox or per-tool authorization gate.

## Cancellation and recovery

Before dispatch, the bridge rechecks OP authorization and records a started event. During active foreground requests, authorization is polled; revocation triggers cancellation and an interruption attempt. This is not an instantaneous per-tool boundary.

Timeouts, transport failures after dispatch and explicit stops may leave partial changes. The bridge never claims that no action occurred merely because an HTTP request failed. It attempts session interruption and records an uncertain/failed outcome for inspection. Completed changes and detached processes require normal server recovery.

On watcher startup/shutdown, recorded sessions are interrupted and undelivered inbox inputs are cancelled. History/ownership remains available, but no request is replayed automatically. After a bridge crash, inspect OpenCode/server state and stale journal/ledger locks before restarting; OpenCode may have kept running independently. If the pinned runtime confirms a session was deleted, recovery clears that ownership entry without replaying work; the next new OP request creates a new conversation. Other interruption failures require inspection.

The Go implementation does not implement a separate restart supervisor or mod installer. OpenCode performs these tasks with full tools, informed by the server instructions. Purpose-built service supervision, backups and release deployment remain separate improvements.
