# Roadmap

## Foundation

- [x] Standalone Go CLI and strict external configuration.
- [x] Fail-closed OP authorization and pre-execution rechecks.
- [x] Explicit whitelist/status workflow with verification and durable audit.
- [x] Optional explicit-command/proposal modes with independent Go validation.
- [x] Windows/Linux GitHub Actions, vulnerability scan and packaging.
- [x] Documentation, MIT license and contribution/security guidance.

## Agent and reliability

- [x] Full OpenCode agent sessions with normal system/web tools for trusted OPs.
- [x] Persistent per-UUID conversations, native question/answer bridging and private replies.
- [x] Authenticated stop controls, conversation reset and best-effort interruption/recovery.
- [x] Pin and test OpenCode 2.0.32's session/tool APIs and separate read-only diagnostics.
- [x] Model/API deadlines, cancellation, failure/refusal handling and bounded responses.
- [x] Provider API-key setup through Minecraft Server Agent and an OpenRouter configuration example.
- [x] Guided setup and independent, actionable connection diagnostics.
- [x] Durable per-player cooldowns, bounded queue, agent-interaction allowance and read-only inference budget.
- [ ] Token/currency accounting and budgets (provider-side limits required meanwhile).
- [ ] Authenticated UUID events and roles beyond OP.
- [ ] Durable job queue and reconciliation of interrupted jobs.
- [x] Agent-driven administration, including restarts/mod installation through normal OpenCode tools and server workspace instructions.
- [ ] Packaged service supervision and additional restart/backup/recovery automation.
- [ ] Windows/Linux service installers without desktop dependencies.
- [ ] Real Minecraft restart/mod-install/backup/recovery integration matrix.
- [ ] Real Minecraft integration matrix and clean Windows pilot.

## Distribution and portfolio

- [x] Make GitHub public at the owner's request.
- [x] GitLab repository/CI, reusing the Go tools.
- [x] GitHub primary, explicit one-way synchronization to GitLab without pipeline loops.
- [ ] Isolated Windows self-managed runner if needed.
- [ ] Manual test-environment deployment and rollback demonstration.
- [ ] Release provenance/signing, install/update docs and public release.
