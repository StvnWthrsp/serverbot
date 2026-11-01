# Roadmap

## Foundation

- [x] Standalone Go CLI and strict external configuration.
- [x] Fail-closed OP authorization and pre-execution rechecks.
- [x] Explicit whitelist/status workflow with verification and durable audit.
- [x] Independently validated planner proposals.
- [x] Windows/Linux GitHub Actions, vulnerability scan and packaging.
- [x] Documentation, MIT license and contribution/security guidance.

## Agent and reliability

- [x] Pin and test OpenCode 2.0.32's headless stateless API.
- [x] Tool-free OpenCode planner producing validated JSON proposals.
- [x] Model/API deadlines, cancellation, failure/refusal handling and bounded responses.
- [x] Provider API-key setup through ServerBot and an OpenRouter configuration example.
- [ ] Global rate/token/cost budgets (provider-side limits required meanwhile).
- [ ] Authenticated UUID events and roles beyond OP.
- [ ] Durable job queue and reconciliation of interrupted jobs.
- [ ] Supervised restart: save/stop, wait, start, readiness and recovery.
- [ ] Windows/Linux service installers without desktop dependencies.
- [ ] Config/mod changes after backup, compatibility and rollback tests.
- [ ] Real Minecraft integration matrix and clean Windows pilot.

## Distribution and portfolio

- [x] Make GitHub public at the owner's request.
- [x] GitLab repository/CI, reusing the Go tools.
- [x] GitHub primary, explicit one-way synchronization to GitLab without pipeline loops.
- [ ] Isolated Windows self-managed runner if needed.
- [ ] Manual test-environment deployment and rollback demonstration.
- [ ] Release provenance/signing, install/update docs and public release.
