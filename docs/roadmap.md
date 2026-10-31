# Roadmap

## Foundation

- [x] Standalone Go CLI and strict external configuration.
- [x] Fail-closed OP authorization and pre-execution rechecks.
- [x] Explicit whitelist/status workflow with verification and durable audit.
- [x] Independently validated planner proposals.
- [x] Windows/Linux GitHub Actions, vulnerability scan and packaging.
- [x] Documentation, MIT license and contribution/security guidance.

## Agent and reliability

- [ ] Pin and test a supported headless OpenCode API version.
- [ ] Read-only/no-tools OpenCode planner producing structured proposals.
- [ ] Model/API timeouts, cancellation, cost budgets and denial handling.
- [ ] Authenticated UUID events and roles beyond OP.
- [ ] Durable job queue and reconciliation of interrupted jobs.
- [ ] Supervised restart: save/stop, wait, start, readiness and recovery.
- [ ] Windows/Linux service installers without desktop dependencies.
- [ ] Config/mod changes after backup, compatibility and rollback tests.
- [ ] Real Minecraft integration matrix and clean Windows pilot.

## Distribution and portfolio

- [ ] Review the feature PR and make the repository public when ready.
- [x] GitLab repository/CI, reusing the Go tools.
- [x] GitHub primary, explicit one-way synchronization to GitLab without pipeline loops.
- [ ] Isolated Windows self-managed runner if needed.
- [ ] Manual test-environment deployment and rollback demonstration.
- [ ] Release provenance/signing, install/update docs and public release.
