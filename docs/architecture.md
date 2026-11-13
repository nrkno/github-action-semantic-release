---
type: Architecture
title: semrel architecture
description: Why semrel exists, internal package structure, and key design decisions including go-git, the hardened alpine container image, idempotency ladder, and shallow clone guard.
tags: [architecture, supply-chain, go-git, alpine, cosign, idempotency]
timestamp: 2026-11-13
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

The module and supported build environments require Go 1.26 or newer. CI pins the
vulnerability scanner version so the scan runs consistently with that toolchain.

---

## Package structure

```
github.com/nrkno/github-action-semantic-release
│
├── cmd/semrel/           Entry point — wires concrete clients, injects into CLI
│
└── internal/
    ├── cli/              Cobra subcommand definitions; accepts interface-injected clients
    ├── conventional/     Conventional commit parser (type, scope, description, breaking)
    ├── env/              Reads and validates GitHub Actions environment variables
    ├── git/              go-git wrapper: FindLatestTag (semver-max across all tag types),
    │                     ListCommitsSinceTag, CreateAnnotatedTag, PushTag (BasicAuth over HTTPS)
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

### Container base image: alpine, unprivileged

The final stage uses a supported `alpine` release (digest-pinned, not a mutable
tag; Renovate keeps tag and digest current). This image:

- Contains a shell (`/bin/sh`) and the standard Alpine userland
- Is required because `entrypoint.sh` (the container's `ENTRYPOINT`) is a shell
  script — a fully static, shell-less minimal base image cannot execute it
- Declares `USER 65532:65532`, and `action.yml` runs it with the runner's own
  uid/gid so it can write the mounted workspace (`.git/`) and `$GITHUB_OUTPUT`

The container runs with `--read-only`, `--cap-drop ALL` and
`--security-opt no-new-privileges`. Only `$GITHUB_WORKSPACE` (read-write),
`$GITHUB_OUTPUT` (the one output file, read-write) and `$GITHUB_EVENT_PATH`
(read-only) are mounted. `$RUNNER_TEMP` is **not** mounted, so even a
compromised image cannot write `$GITHUB_ENV` or `$GITHUB_PATH` to persist into
later steps of the caller's job.

The image is built for `linux/amd64` and `linux/arm64` by cross-compiling on the
build platform. The attack surface is a shell plus the Alpine userland; it is
mitigated by two controls:

1. **Digest-pinned image plus signature verification before it runs.** See
   [Pinning and verification](#pinning-and-verification) below.
2. **The build pipeline itself.** The image is produced by a reviewed source tree,
   built by GitHub Actions CI and signed with the CI job's own OIDC identity — see
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

### Version baseline detection: semver-max across all tags

`FindLatestTag` collects all git tags matching `cfg.TagPrefix`, parses each as semver,
discards non-parseable names, and returns the tag with the highest semver value. Tag type
(annotated vs lightweight) is not a factor in selection.

**Why:** The previous strategy (`FindLatestAnnotatedTag`) preferred annotated tags and
fell back to lightweight only when zero annotated tags existed. This was correct for
a clean install but broke for repos migrated from `codfish/semantic-release`: codfish
created annotated `vMAJOR.MINOR` alias tags for every release, so any migrated repo has
annotated aliases from its entire pre-migration history. These aliases permanently shadow
post-migration releases (which produce only lightweight `vMAJOR.MINOR.PATCH` tags).

**Sort key:** Semver ordering is a total order and is immune to clock skew, timezone
normalization, and the `Author.When` vs `Committer.When` ambiguity that affected the
previous timestamp-based sort.

**Tiebreaker:** Masterminds/semver treats `v4.0` and `v4.0.0` as equal (`Compare() == 0`).
Because `sort.Slice` is not stable, a secondary tiebreaker is applied: prefer the tag with
more dot-separated components (`strings.Count(name, ".")` descending), then `name`
descending lexicographically as the final tiebreaker. This ensures `v4.0.0` consistently
beats `v4.0`.

**ADR:** See [ADR-001](architecture/adr-001-version-baseline-detection.md).

### SHA comparison

The tag SHA check in rung 2 compares `tag.TargetSHA()` (the commit an annotated tag
points to) against the HEAD commit SHA from the direct `HeadSHA()` lookup, without
enumerating history. A mismatch means a different commit was tagged with the same calculated version, which is a
genuine conflict (exit 1). This guards against two parallel release runs computing
the same next version for different commits.

Commit range membership follows graph ancestry, not commit timestamps. PR lint excludes
every commit reachable from its base before loading range commits. Release enumeration
is "reachable from HEAD and not reachable from the selected tag's commit", whether or not
the tag is an ancestor of HEAD. The release tag points at an off-branch digest commit, so
only the commits since the history it was built on are counted; a tag with unrelated
history excludes nothing. `notify` uses the same rule between the previous and released
tag (`ListCommitsBetweenTags`). Tag selection remains highest-semver across tags, and
releasing from several long-lived branches is not supported (commits released under a
non-baseline tag are counted again). `notify` picks the previous tag by the author date of
tag targets, a known limitation. Tagless bootstrap includes all HEAD history.
See [release-image-pinning ADR-001](/docs/architecture/release-image-pinning/decisions/adr-001-off-branch-digest-tag-with-ancestry-ranges.md).
Ancestry discovery may traverse full base history: this bounds full commit materialization,
not all graph work or underlying object I/O.

### Shallow clone requirement

semrel requires a full clone (`fetch-depth: 0`). The `release`, `lint`, and `notes`
subcommands all walk commit history back to the previous tag. A shallow clone truncates
that history.

A shallow clone causes `OpenRepo()` to return `ShallowRepoError`. The startup code in
`cmd/semrel/main.go` catches this error, logs a diagnostic, and exits with code 2 —
`FindLatestTag` is never called.

---

## Build and container pipeline

`.github/workflows/release.yml` (push to `main`) has three jobs. The image is
built and signed **before** the tag exists, so the tagged commit can carry the
image digest:

```
push to main
  ├─ plan     ./semrel release --dry-run          (is there a release? which tag?)
  ├─ image    docker build + push (amd64, arm64)  (ghcr.io/nrkno/github-action-semantic-release:<tag>)
  │           cosign sign (keyless, OIDC)         -> image digest
  └─ release  commit "chore(release): pin image digest" (action.yml now holds @sha256:<digest>)
              ./semrel release                    (tags THAT commit, creates the GitHub Release)
```

The release tag therefore points at a commit that is a child of the `main`
commit but is not on any branch (GitHub shows a "does not belong to any branch" banner
on it). The digest commit uses the `main` commit's date for author and committer, so a rerun
with the same image recreates the same commit. After tagging, the check step asserts that
the tag's commit is a direct child of `github.sha` and contains the signed digest exactly
once. A commit SHA pin of that commit pins the image. The image's `COMMIT` build argument and
`org.opencontainers.image.revision` label are the `main` commit (the parent), not the tagged
digest commit.
`action.yml` on `main` holds an all-zero placeholder digest; using an unreleased
commit fails with a clear error.

The `plan` and `release` jobs build semrel from source on every push to `main`. This means the
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

`action.yml` is a **composite action** (`runs.using: composite`). The image
reference is a **digest** (`ghcr.io/nrkno/github-action-semantic-release@sha256:…`)
written into `action.yml` by the release workflow (see above) — not a tag
resolved at run time. The commit SHA a consumer pins therefore also pins the
image; there is no window in which a mutable registry tag could be re-pointed.

Before `docker run` executes, a `verify` step runs `cosign verify` on that
digest and accepts only a signature from this repository's release workflow on
`main`:

```bash
cosign verify "$ACTION_IMAGE" \
  --certificate-identity "https://github.com/nrkno/github-action-semantic-release/.github/workflows/release.yml@refs/heads/main" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```

This is enforced at every run: an unsigned image, or one signed by another
workflow or ref (for example a workflow on a feature branch), makes the
composite action fail before `docker run`.

| Layer | Mechanism | What it protects against |
| ----- | --------- | ------------------------ |
| **Git layer** | `uses: …@<commit-SHA>` | Attacker force-pushes a tag to a malicious commit |
| **Container layer** | Image digest in `action.yml` plus `cosign verify` of an exact workflow identity | An image not built and signed by `release.yml` on `main` is refused; a tag cannot be re-pointed |

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
- **A verified image can still act within its mounts**: cosign verification proves
  the image's *provenance* — it says nothing about what a legitimately-built image
  does once it runs. The container runs unprivileged, read-only and without
  capabilities, with only the workspace, `$GITHUB_OUTPUT` and the event payload
  mounted, and receives an allowlisted set of job variables (including the token)
  via `--env-file`. Consumers should still treat action
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
IMAGE=$(grep -o 'ghcr.io/[^"]*@sha256:[0-9a-f]\{64\}' action.yml | head -1)
cosign verify "$IMAGE" \
  --certificate-identity="https://github.com/nrkno/github-action-semantic-release/.github/workflows/release.yml@refs/heads/main" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com"
```

Each release prints its commit SHA in the workflow summary and on the
[releases page](https://github.com/nrkno/github-action-semantic-release/releases).
