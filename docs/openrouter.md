# OpenRouter through OpenCode

ServerBot continues to call OpenCode's pinned stateless API. OpenRouter replaces the OpenCode-hosted inference provider; ServerBot does not call OpenRouter directly. The Go adapter and operation policy remain the same.

The example ServerBot config selects `provider_id: openrouter` and `model_id: deepseek/deepseek-v4.1-flash`. This is a paid OpenRouter model, chosen for a small bounded evaluation; configure your own account budget before enabling it. Model availability and pricing may change.

Use `examples/opencode/openrouter.json` as the dedicated OpenCode configuration. It denies tools, requests a strict two-field JSON schema, requires providers to support the parameters, disables reasoning, and caps output at 256 tokens. Supply `OPENROUTER_API_KEY` only to the OpenCode process. The example contains no key. Go still independently validates every proposal; schema constraints do not replace authorization or validation.

## OCI-managed evaluation

`tools/oci-openrouter-eval.py` runs on an OCI instance with the Python OCI SDK and instance-principal permission to list secret metadata and read the intended secret bundle. It discovers the active secret named `openrouter-api-key` in the instance's compartment, retrieves the CURRENT bundle, decodes it in memory and starts isolated OpenCode with the key in its environment. It never prints the key or transfers it to the developer workstation, source files, ServerBot's evaluator, or CI variables.

The launcher creates temporary config/data directories with deny-all permissions and the bounded OpenRouter settings. OpenCode's startup password is captured privately and passed to the evaluator; only synthetic cases/results are emitted. The runtime is stopped and the temporary directories are removed after evaluation. An already-installed OpenCode service is not modified.

Build the proposal-only evaluator for the instance architecture (OCI `n8n-a1` is Linux arm64):

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/serverbot-evaluate ./tools/evaluate
```

Copy the evaluator and Python launcher to the instance. Install OpenCode 2.0.32 in a separate directory, preserving any other installed version. On the instance, use the Python interpreter with the OCI SDK installed:

```sh
chmod 700 ./serverbot-evaluate
python tools/oci-openrouter-eval.py \
  --opencode /absolute/path/to/opencode-2.0.32 \
  --evaluate /absolute/path/to/serverbot-evaluate \
  --model deepseek/deepseek-v4.1-flash
```

Redirect stdout to a report file if desired. Progress goes to stderr. The command makes 18 model calls at most, with a 45-second per-case deadline, and stops on the first transport/provider error. Non-passing runs return a nonzero status after writing their report. No RCON, Minecraft server files, administration commands or live player data are used.

This is an explicit live evaluation, outside normal CI. GitHub/GitLab continue to use a real OpenCode runtime with a local fake provider, so CI needs no cloud key or model credits. The OCI launcher is not yet a production service installer; a Windows pilot, service supervision and secret-refresh lifecycle remain future work.

## Recorded result

On 2026-11-01 the OCI-backed run passed **18/18 cases** through OpenCode 2.0.32 and OpenRouter's `deepseek/deepseek-v4.1-flash`: seven expected whitelist/status proposals and eleven expected refusals, including unsupported administration, ambiguous requests, multi-operation requests and prompt-injection attempts. There were no unexpected accepted proposals. Median latency was 453.5 ms; the slowest case took 2,620 ms.

See the [machine-readable report](evaluations/2026-11-01-openrouter.json). This is one small synthetic run, not a general security guarantee or a live Minecraft deployment. Exact provider cost was not measured. Calls were capped at 256 output tokens; no automatic model fallback was configured.

References: [OpenRouter structured outputs](https://openrouter.ai/docs/guides/features/structured-outputs), [OCI secret retrieval](https://docs.oracle.com/en-us/iaas/Content/secret-management/Tasks/get-secrets-contents.htm).
