# GitHub and GitLab CI/CD

GitHub is the primary repository: https://github.com/StvnWthrsp/serverbot.
GitLab holds a secondary copy for pipeline execution: https://gitlab.com/stvnwthrsp/serverbot.
GitHub is public; the GitLab copy is private. Contributions and merges happen on GitHub; do not independently merge changes in GitLab.

## Pipeline responsibilities

| Check | GitHub Actions | GitLab CI |
| --- | --- | --- |
| Formatting, vet, race tests, build | Native Windows and Linux | Linux container |
| Known Go vulnerabilities | Pinned govulncheck | Same pinned govulncheck |
| OpenCode API/no-tools contract | Pinned runtime, local fake model | Same contract helper |
| Windows/Linux amd64/arm64 packaging | Packaging smoke test | Downloadable job artifacts |
| Coverage | Job artifacts | Job artifacts |
| Version tags | Checks and draft GitHub release | Checks and versioned package artifacts |

GitLab uses `.gitlab-ci.yml`: verification runs in parallel, then packaging depends on checks, vulnerability scanning and the OpenCode contract succeeding. Go module/build caches are keyed by `go.mod`; artifacts expire after 14 days. Rules switch from branch to merge-request pipelines when a GitLab MR is open, avoiding duplicates. New commits cancel superseded interruptible jobs. Both providers call the same Go tools.

GitLab's initial pipeline uses a versioned official Go Linux image and hosted Linux runners. It does not perform native Windows tests; those remain in GitHub Actions. Cross-compilation is not a substitute for native runtime tests.

## Authentication and synchronization

Authenticate using `gh auth login` and `glab auth login`. Configure the GitLab remote and repository-local credential helpers (no tokens are stored in source):

```sh
git remote add gitlab https://gitlab.com/stvnwthrsp/serverbot.git
git config credential.https://github.com.helper '!gh auth git-credential'
git config credential.https://gitlab.com.helper '!glab auth git-credential'
```

Push the current feature branch to both providers:

```sh
go run ./tools/sync
```

This pushes GitHub first and GitLab second, without force pushes, branch deletion or tag propagation. It preserves GitHub as the branch upstream. It is an explicit developer command, not a background mirror. If one push fails, fix it and rerun; the first push may already have succeeded. No cross-provider access tokens or mirroring webhooks are installed, so pipelines cannot trigger synchronization loops.

After merging on GitHub, update the GitLab default branch explicitly:

```sh
git fetch origin main
git push gitlab refs/remotes/origin/main:refs/heads/main
```

Push reviewed version tags to both remotes explicitly when ready for a release. The sync command refuses the default branch to preserve the feature-branch workflow.

## GitLab account prerequisites

GitLab.com may require account/billing identity verification before hosted runners execute jobs. The pipeline cannot bypass that account requirement. If jobs are blocked, complete GitLab's verification flow or register an isolated self-managed runner with a matching tag. Do not use the live Minecraft machine as an unrestricted contributor runner.

Project CI configuration can be linted through GitLab's CI Lint API before running jobs. Syntax validation does not establish that runners are available or that the pipeline has passed. Neither provider deploys to a live Minecraft server yet.
