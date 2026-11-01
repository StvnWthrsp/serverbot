# OpenRouter through OpenCode

Minecraft Server Agent uses OpenCode as its runtime and OpenRouter as an inference provider. You choose the OpenRouter model; Minecraft Server Agent does not require a particular model.

1. Follow the [OpenCode setup](opencode.md#setup), using `examples/opencode/openrouter.json` as the dedicated runtime configuration.
2. In your private `minecraft-server-agent.json`, set `opencode.provider_id` to `openrouter` and `opencode.model_id` to the model identifier from your OpenRouter account (including its publisher prefix).
3. Provide your API key to the Minecraft Server Agent process through `OPENROUTER_API_KEY`, using your shell's private prompt or service's environment settings. Keep the key out of command arguments and checked-in files.
4. Connect it to the running OpenCode server:

```sh
minecraft-server-agent connect -config minecraft-server-agent.json -key-env OPENROUTER_API_KEY
```

OpenCode saves the credential. Remove the variable from the Minecraft Server Agent environment after setup. Alternatively, use `opencode auth login --standalone` and select OpenRouter under the same dedicated account and directories, then restart the headless runtime. `MSA_OPENCODE_PASSWORD` is the separate password for the local OpenCode HTTP server; it is not the OpenRouter API key.

Preview a proposal before enabling chat administration:

```sh
minecraft-server-agent plan -config minecraft-server-agent.json -message "minecraft-server-agent, please whitelist Friend_1"
```

The example denies tools, caps output at 256 tokens and requests a strict two-field JSON schema. Choose a model that supports these parameters, or adapt the provider configuration for your model. Go independently validates every proposal regardless of provider settings. Configure account spending limits in OpenRouter; Minecraft Server Agent does not yet enforce a global cost budget.

Other providers use the same Minecraft Server Agent configuration and [API-key or OAuth setup](opencode.md#provider-authentication), with their own provider/model identifiers.
