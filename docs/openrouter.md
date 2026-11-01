# OpenRouter through OpenCode

Minecraft Server Agent uses OpenCode as its runtime and OpenRouter as an inference provider. You choose the OpenRouter model; Minecraft Server Agent does not require a particular model.

1. Follow the [OpenCode setup](opencode.md#setup), using `examples/opencode/openrouter.json` as the runtime configuration. It permits the full tool loop.
2. Use `config.agent.example.json` or the setup wizard's default `agent` mode. In your private configuration, set `opencode.provider_id` to `openrouter` and `opencode.model_id` to a tool-capable model identifier from your account (including its publisher prefix).
3. Provide your API key to the Minecraft Server Agent process through `OPENROUTER_API_KEY`, using your shell's private prompt or service's environment settings. Keep the key out of command arguments and checked-in files.
4. Connect it to the running OpenCode server:

```sh
minecraft-server-agent connect -config minecraft-server-agent.json -key-env OPENROUTER_API_KEY
```

OpenCode saves the credential. Remove the variable from the Minecraft Server Agent environment after setup. Alternatively, use `opencode auth login --standalone` and select OpenRouter under the same dedicated account and directories, then restart the headless runtime. `MSA_OPENCODE_PASSWORD` is the separate password for the local OpenCode HTTP server; it is not the OpenRouter API key.

Optionally check provider/model connectivity without executing administration:

```sh
minecraft-server-agent doctor -config minecraft-server-agent.json -check-model
```

The example allows tools, sets an adjustable 32-step Build agent limit and caps each model response at 4096 output tokens. It does not request a proposal schema. Choose a tool-capable model and adapt these settings to its capabilities. Run `minecraft-server-agent run` to forward OP chat to the full agent. Configure account spending limits in OpenRouter; an agent interaction may make multiple model calls and Minecraft Server Agent does not enforce a currency budget.

Other providers use the same Minecraft Server Agent configuration and [API-key or OAuth setup](opencode.md#provider-authentication), with their own provider/model identifiers.
