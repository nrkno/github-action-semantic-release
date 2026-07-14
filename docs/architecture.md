---
type: Architecture
title: semrel architecture
description: Why semrel exists, internal package structure, and key design decisions including go-git, the alpine container image, idempotency ladder, and shallow clone guard.
tags: [architecture, supply-chain, go-git, alpine, cosign, idempotency]
timestamp: 2026-07-01
---

# Architecture

## Why semrel exists

In 2024, the `codfish/semantic-release` GitHub Action was identified as a supply-chain
risk: the upstream maintainer's account was compromised and a malicious version was
briefly published. NRK teams consuming that action were exposed.

**semrel** replaces it with a first-party Go CLI that:

- Has no external JavaScript runtime dependency
- Uses only `GITHUB_TOKEN` — no external secrets or service accounts
- Is built from source in CI before use (no pinned third-party action required)
- Publishes a cosign-signed container image for teams that prefer the container path

---

## Package structure

```
github.com/nrkno/semrel
│
├── cmd/semrel/           Entry point — wires concrete clients, injects into CLI
│
└── internal/
    ├── cli/              Cobra subcommand definitions; accepts interface-injected clients
    ├── conventional/     Conventional commit parser (type, scope, description, breaking)
    ├── env/              Reads and validates GitHub Actions environment variables
    ├── git/              go-git wrapper: FindLatestAnnotatedTag, ListCommitsSinceTag,
    │                     CreateAnnotatedTag, PushTag (BasicAuth over HTTPS)
    ├── github/           google/go-github REST client wrapper: GetReleaseByTag,
    │                     CreateRelease (with 422→re-GET idempotency), ListPRsForCommit,
    │                     SearchPRsForCommit, PostPRComment, FindPRComment
    ├── notes/            Markdown release notes generator (groups commits by type)
    ├── output/           Writes key=value pairs to the GITHUB_OUTPUT file
    └── semver/           Version parsing, bump-type detection (major/minor/patch/none),
                          next-version arithmetic
```

### Dependency direction

```
cmd/semrel → internal/cli
internal/cli → conventional, env, git, github, notes, output, semver
internal/notes → conventional
internal/github → (go-github, oauth2)
internal/git → (go-git)
internal/semver → (Masterminds/semver)
```

No internal package imports `internal/cli`. There are no circular imports.

---

## Design decisions

### go-git vs git binary

go-git is used instead of shelling out to the `git` binary. This eliminates a runtime
dependency on a git installation entirely — semrel never shells out to `git`, so the
container image does not need a git binary regardless of which base image it uses.

Trade-off: go-git's annotated-tag and push APIs are lower-level than the git CLI.
The `internal/git` package wraps this complexity behind a narrow interface
(`GitClient`) so the rest of the codebase is insulated from go-git specifics.

### Container base image: alpine, running as root

The final stage uses `alpine:3.19` (digest-pinned, not a mutable tag). This image:

- Contains a shell (`/bin/sh`) and the standard Alpine userland
- Is required because `entrypoint.sh` (the container's `ENTRYPOINT`) is a shell
  script — a fully static, shell-less minimal base image cannot execute it
- Runs as **root** — deliberately, not by oversight

**Why root:** GitHub Actions mounts `$GITHUB_WORKSPACE` (the checked-out repo,
including `.git/`) and `$RUNNER_TEMP` (which holds `$GITHUB_OUTPUT`) from the runner
host, where they are owned by the runner's root user. A non-root container user
cannot write tags, push commits, or write `$GITHUB_OUTPUT` against files it does not
own. Running as root is the pragmatic choice given how the composite action mounts
the workspace — the same constraint that affects most container-based GitHub Actions
that need to write back into the repository.

**What this means:** the container has a real attack surface — a shell, package
manager, and standard userland — rather than the minimal footprint of a
scratch-based or shell-less image. This is accepted and mitigated by two controls
rather than eliminated:

1. **Cosign signature verification before the image ever runs.** The composite
   action's `verify` step calls `cosign verify` against this repo's OIDC identity
   before `docker run` executes — see [Pinning and verification](#pinning-and-verification)
   below. An unsigned or wrongly-signed image is refused; nothing built by anyone
   other than this repository's own CI ever runs.
2. **The build pipeline itself.** The image is produced by a reviewed source tree,
   built by GitHub Actions CI (not a local or third-party build), and signed with
   the CI job's own OIDC identity — the same chain of custody documented in
   [Build and container pipeline](#build-and-container-pipeline).

The Go binary is compiled with `CGO_ENABLED=0` to produce a fully static binary,
independent of the base image's libc.

### GITHUB_TOKEN only

semrel intentionally accepts only `GITHUB_TOKEN`. There is no support for PATs,
GitHub App JWTs, or SSH keys. This keeps the permission model simple: the token
is scoped to the repository and expires after the workflow run.

Git push authentication uses HTTP Basic Auth with username `x-access-token` and
password `GITHUB_TOKEN` — the [documented GitHub pattern](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/about-authentication-to-github#authenticating-with-the-api).

### Idempotency ladder

The `release` subcommand applies a three-rung idempotency check before creating
anything. This prevents double-releases when a workflow is re-run:

1. **Release exists** for the computed tag → write output fields, exit 0 (no-op)
2. **Tag exists, no release** → create GitHub Release from the existing tag SHA;
   if the existing tag SHA ≠ HEAD SHA, exit 1 (conflict — a different commit has
   already been tagged with this version)
3. **Neither exists** → create annotated tag, push, create GitHub Release

Rung 2 includes a `422 already_exists` retry: if `CreateRelease` returns 422,
semrel calls `GetReleaseByTag` to retrieve the release that was created concurrently
and proceeds as if rung 1 had matched.

### SHA comparison

The tag SHA check in rung 2 compares `tag.TargetSHA()` (the commit a annotated tag
points to) against the HEAD commit SHA from `ListCommitsSinceTag(nil)`. A mismatch
means a different commit was tagged with the same calculated version, which is a
genuine conflict (exit 1). This guards against two parallel release runs computing
the same next version for different commits.

### Shallow clone requirement

semrel requires a full clone (`fetch-depth: 0`). The `release`, `lint`, and `notes`
subcommands all walk commit history back to the previous annotated tag. A shallow
clone truncates that history, causing `FindLatestAnnotatedTag` to return `nil` and
producing incorrect version calculations.

The startup code in `internal/git` detects a shallow repository and exits with
code 2 (system error) to surface the misconfiguration clearly rather than silently
producing a wrong result.

---

## Build and container pipeline

```
push to main
  └─▶ release job
        ├─ go build -o semrel ./cmd/semrel   (builds from source — no bootstrap dep)
        ├─ ./semrel release                  (creates tag + GitHub Release)
        └─ if released: publish-image job
              ├─ docker build (multi-stage, alpine, root)
              ├─ docker push  (ghcr.io/nrkno/github-action-semantic-release:<tag>)
              └─ cosign sign  (keyless, OIDC from GitHub Actions)
```

The release job builds semrel from source on every push to `main`. This means the
self-release pipeline never depends on a previously published container — avoiding
the bootstrap problem that affected the upstream action.

---

## Security Model

### Why this project exists

The `codfish/semantic-release` GitHub Action was identified as a supply-chain risk
after the upstream maintainer's account was compromised and a malicious version was
briefly published. semrel replaces it with a first-party binary built from source in
CI, eliminating the dependency on a third-party action.

### Pinning and verification

`action.yml` is a **composite action** (`runs.using: composite`), not a Docker
container action. The `run` step resolves the image reference dynamically at
run time:

```bash
docker run --rm ... "ghcr.io/${{ github.action_repository }}:${{ github.action_ref }}"
```

`github.action_ref` is whatever ref the consumer wrote in their own `uses:` line —
a version tag (`v1.2.3`) or a full 40-character commit SHA. The image is tagged
with both forms on every release, so either pinning style resolves to the correct
image automatically. This is a deliberate difference from a Docker container
action (`runs.using: docker`), which bakes one fixed image reference directly
into `action.yml`'s `image:` field and must be re-pointed by a chore-commit PR
on every release. This repository used the container-action mechanism until the
self-referential composite action landed (see `docs/log.md`).

Before `docker run` executes, a `verify` step runs `cosign verify` against the
resolved image reference:

```bash
cosign verify "ghcr.io/${ACTION_IMAGE_REPO}:${ACTION_IMAGE_TAG}" \
  --certificate-identity-regexp "https://github.com/nrkno/github-action-semantic-release/.github/workflows/.*" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```

This is enforced at every run, not merely documented as a recommendation: if the
image is unsigned, signed by a different identity, or the signature doesn't
validate, `cosign verify` exits non-zero and the composite action fails before
`docker run` ever executes. On every release, the CI pipeline signs the image
keylessly against the NRK GitHub Actions OIDC identity.

| Layer | Mechanism | What it protects against |
| ----- | --------- | ------------------------ |
| **Git layer** | `uses: …@<commit-SHA>` | Attacker force-pushes a tag to a malicious commit |
| **Container layer** | `cosign verify` step, enforced before every `docker run` | An image not built and signed by this repository's own CI is refused at run time |

#### Git layer — commit SHA pinning

Git tags are mutable: anyone with push access to this repository can force-push a
tag to a different commit. A pinned tag (`@v1.2.3`) is not a security guarantee.

Pinning by **commit SHA** (`@abc1234…`) is immutable — the SHA is a cryptographic
hash of the commit content. GitHub will refuse to serve a different commit for a
given SHA.

### What these layers do NOT protect against

- **Compromised build pipeline**: if the CI workflow itself is tampered with during
  a release run, both the binary and the image could be replaced before signing.
- **Compromised Go dependencies**: a malicious dependency could be introduced via a
  supply-chain attack on a dependency of semrel. Dependabot and `govulncheck` reduce
  but do not eliminate this risk.
- **Compromised GitHub Actions runners**: a compromised runner environment is outside
  the scope of any pinning strategy.
- **A verified image still runs as root with write access to the job's environment**:
  cosign verification proves the image's *provenance* (this repo's CI built and
  signed it) — it says nothing about what a legitimately-built image is permitted
  to do once it runs. The container executes as root with `$GITHUB_WORKSPACE` and
  `$RUNNER_TEMP` (which holds `$GITHUB_OUTPUT`/`$GITHUB_ENV`) bind-mounted in, and
  receives job variables via `--env-file`. Consumers should still treat action
  outputs as untrusted-ish string data: avoid unsafe interpolation of outputs into
  later `run:` steps, e.g.
  `run: echo ${{ steps.x.outputs.notes }}` (unquoted, shell-interpolated) can be
  used to inject shell metacharacters if `notes` ever contains attacker-influenced
  commit-message text. Prefer passing outputs through `env:` and referencing the
  environment variable inside the script instead of interpolating the expression
  directly into `run:`.

### Recommendation

Pin by **commit SHA** at the git layer. Cosign verification of the container image
now happens automatically on every run (see above) — no manual step is required,
but consumers can additionally verify independently:

```bash
cosign verify ghcr.io/nrkno/github-action-semantic-release:v1.2.3 \
  --certificate-identity-regexp="https://github.com/nrkno/github-action-semantic-release/.*" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com"
```

Each release prints its commit SHA in the workflow summary and on the
[releases page](https://github.com/nrkno/github-action-semantic-release/releases).
