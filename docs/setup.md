# First-run setup

The executable is `minecraft-server-agent` (`minecraft-server-agent.exe` on Windows). In PowerShell, prefix commands with `.\`; in a Unix shell, use `./` unless the executable is on PATH.

```sh
./minecraft-server-agent setup
```

Press Enter to accept each displayed default. Setup asks for the Minecraft directory, loopback RCON address, planner, optional OpenCode URL/provider/model, and [request limits](request-limits.md). Relative Minecraft paths entered in setup refer to your current directory; the wizard saves an absolute path. Audit/ledger paths are relative to the resulting configuration file.

The default `agent` mode forwards OP requests to OpenCode's full system/web tool loop. It uses a 900-second request deadline and a persistent conversation per OP UUID. The wizard explains that OPs must be trusted with the runtime account's machine access. See [agent behavior](agent.md), [OpenCode](opencode.md) and [OpenRouter](openrouter.md).

Optional `commands` mode needs no inference provider and accepts explicit whitelist/status commands. The optional `opencode` mode uses a restricted proposal planner; it is not the full agent workflow. Existing configurations retain their selected mode until edited.

Setup never asks you to type a password or API key. Supply `MSA_RCON_PASSWORD` through your process/service environment. For OpenCode, also supply its local startup password as `MSA_OPENCODE_PASSWORD`. If importing a provider API key, enter only the name of the environment variable containing it, such as `OPENROUTER_API_KEY`; OpenCode saves the credential. Leave that answer blank to reuse an existing connection, including OAuth. For OAuth, use `opencode auth login --standalone` under the same dedicated account and configuration/data directories, then restart the headless runtime.

The wizard writes a validated `minecraft-server-agent.json` with exclusive creation and restrictive Unix permissions. It refuses existing files/symlinks and writes nothing on incomplete/invalid input. Use `setup -config another-name.json` for another configuration in an existing directory. On Windows, restrict access through account/directory ACLs.

Enable `online-mode=true` and `enable-rcon=true` yourself in Minecraft's `server.properties`. Match `rcon.port` to the configured loopback address. Start Minecraft once so its logs and permissions files exist. Restrict the Minecraft RCON listener using the host firewall; a client loopback address does not restrict the listener.

After saving, setup can check the files and connections. A failed connection does not delete the saved configuration: follow the reported instructions and rerun:

```sh
./minecraft-server-agent doctor
```

This checks files/authentication settings, RCON and the pinned OpenCode version independently. Normal setup/doctor make no inference calls. To verify provider credentials and model access with one budgeted inference attempt:

```sh
./minecraft-server-agent doctor -check-model
```

This always uses the separate stateless diagnostic endpoint, expects a status proposal and executes no administration. It does not demonstrate the agent's tools. Then start the watcher:

```sh
./minecraft-server-agent run
```

Run it before sending chat requests; it starts at the current end of the log. Automatic service installation remains planned.

For agent mode, put server operating context (startup/stop commands, loader versions, backups and recovery) in the Minecraft directory's `AGENTS.md`. Native questions and follow-ups use the same conversation; use `minecraft-server-agent /stop` to interrupt and `/new` to reset it.

## Naming changes

The Go module and repositories are `minecraft-server-agent`. The executable/command directory is `minecraft-server-agent`, the default configuration is `minecraft-server-agent.json`, environment variables use `MSA_`, and chat requests begin with `minecraft-server-agent`. Update existing process/service settings accordingly; an existing configuration can still be supplied with `-config`. Preserve its audit path to retain cooldown/deduplication history. Configurations without a `limits` object receive the documented defaults.
