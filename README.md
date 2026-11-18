# semrel

[![Go version](https://img.shields.io/badge/go-1.26-00ADD8?logo=go)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![CI](https://github.com/nrkno/github-action-semantic-release/actions/workflows/ci.yml/badge.svg)](https://github.com/nrkno/github-action-semantic-release/actions/workflows/ci.yml)

**semrel** is NRK's in-house semantic-release CLI — a single Go binary that enforces
[Conventional Commits](https://www.conventionalcommits.org/), computes the next semver,
creates GitHub Releases, and notifies merged PRs. It replaces the supply-chain-compromised
`codfish/semantic-release` Action.

The container image is published to GHCR and signed with
[cosign keyless](https://docs.sigstore.dev/cosign/signing/overview/) on every release.

---

## Security

> ⚠️ **Always pin by commit SHA, not tag.**

Git tags are mutable. An attacker with push access to this repository could
force-push a tag to a malicious commit. Pinning by commit SHA gives you an
immutable reference that cannot be silently changed.

```yaml
- uses: nrkno/github-action-semantic-release@<COMMIT_SHA>
  with:
    subcommand: release
    token: ${{ secrets.GITHUB_TOKEN }}
```

Starting with the digest-pinned release, the tagged commit contains the image
digest in `action.yml`, so pinning that commit SHA pins the exact container image
that runs. The action verifies the image's cosign keyless signature on every run
and accepts only a signature made by this repository's `release.yml` workflow on
`main`. To verify by hand, read the digest from `action.yml` at the commit you pinned:

```bash
IMAGE=$(grep -o 'ghcr.io/[^"]*@sha256:[0-9a-f]\{64\}' action.yml | head -1)
cosign verify "$IMAGE" \
  --certificate-identity="https://github.com/nrkno/github-action-semantic-release/.github/workflows/release.yml@refs/heads/main" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com"
```

Use the commit SHA printed in the release workflow summary or the commit
pointed to by the tag on the [releases page](https://github.com/nrkno/github-action-semantic-release/releases).
Do not use `@main`, a PR SHA, or the `main` commit from the image label: those
refs carry an unset digest and the action will stop.

---

## Quick start

For a repository that consumes semrel, use a published action. Replace
`<COMMIT_SHA>` with its released commit SHA:

```yaml
on:
  push:
    branches: [main]

permissions:
  contents: write
  pull-requests: read

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7.0.1
        with:
          fetch-depth: 0
          fetch-tags: true
          persist-credentials: false
      - uses: nrkno/github-action-semantic-release@<COMMIT_SHA>
        with:
          subcommand: release
```

> **⚠️ `fetch-depth: 0` is required.**
> semrel reads the full commit and tag history to determine the previous version and build
> the commit range. A shallow clone (`fetch-depth: 1`, the GitHub Actions default) causes
> semrel to exit with code `2` (system error). Always set `fetch-depth: 0`.

---

## Runtime migration

- Use a Linux `amd64` or `arm64` runner with Bash, Docker and access to GHCR
  and Sigstore. On self-hosted runners, run the runner service as a non-root
  user that can use Docker. Docker must be able to bind-mount the runner's local paths.
- The container now runs with the runner's UID and GID. The checkout and step
  output must be writable by that user. The container sees the workspace, its
  own `$GITHUB_OUTPUT` and a read-only event payload. It does not see the rest of
  `$RUNNER_TEMP`. Put files semrel needs in the workspace. The container's `/tmp`
  is writable, temporary storage.
- The `token` input still defaults to `github.token`. semrel uses it directly
  for GitHub API calls and for pushing tags over HTTPS; checkout does not need
  to persist its credentials.

## Releasing this action

The [release workflow](.github/workflows/release.yml) builds semrel from source
to plan and publish the release. It does not call an older action and does not
need a published image to bootstrap. When the plan detects a releasable bump,
the workflow pushes and signs the image, then tags a child commit containing
that exact image digest. That child is intentionally not on `main`.

For the transition from `v0.14.5`, wait for that workflow to publish the first
digest-pinned release. Then verify its signed image and update the consumer
pins and lockfiles, including this repository's notifier. Do not substitute a
pre-release branch or commit for the released pin. See the
[maintainer runbook](docs/playbook.md#bootstrap-the-digest-pinned-action).

---

## Subcommands

| Subcommand | Description                                                              |
| ---------- | ------------------------------------------------------------------------ |
| `lint`     | Validate conventional commits in the current event's relevant range. Lint rules can be overridden via `.semrelrc.yml` at the repo root — see [docs/configuration.md](/docs/configuration.md#lint-configuration-file-semrelrcyml). |
| `release`  | Compute next semver, create annotated tag, push, create GitHub Release  |
| `notify`   | Post a deduplicated release comment on the merged PR                    |
| `notes`    | Generate Markdown release notes from commits and linked PRs              |

---

## Inputs & Outputs

<!-- The generated examples below do not select a published digest-pinned
release. Use the hand-written Quick start for a release workflow. -->
<!-- autodoc start -->
## semrel
Conventional-commit linter, semantic versioner, GitHub Release creator, PR notifier, and release notes generator — in one self-owned Go binary.

### Required Inputs

- **subcommand**: Subcommand to run: lint, release, notify, or notes.   <br> (Default: `None`, type: string)

### Optional Inputs

- **token**: GitHub token used for API calls and pushing tags. (Default: `${{ github.token }}`, type: string)
- **dry-run**: Skip tag creation, push, and release creation. Prints what would happen.   <br> (Default: `false`, type: string)
- **working-directory**: Path to the git repository root. Defaults to workspace root. (Default: `.`, type: string)


### Outputs

- **released**: 'true' if a new release was published, 'false' otherwise. (type: unknown)
- **version**: The released version, e.g. '1.2.3'. (type: unknown)
- **tag**: The released git tag, e.g. 'v1.2.3'. (type: unknown)
- **bump**: Version bump type: major, minor, patch, or none. (type: unknown)
- **notes**: Rendered markdown release notes. (type: unknown)
- **sha**: HEAD commit SHA at time of release. (type: unknown)
- **major_version**: Major version component (integer). (type: unknown)
- **minor_version**: Minor version component (integer). (type: unknown)
- **patch_version**: Patch version component (integer). (type: unknown)
- **major**: Major version component (integer). Alias for major_version. (type: unknown)
- **minor**: Minor version component (integer). Alias for minor_version. (type: unknown)
- **patch**: Patch version component (integer). Alias for patch_version. (type: unknown)

### Simple example usage

```yaml
---
name: Example Workflow using this Action
on:
  pull_request:

jobs:
  example-job:
    name: Example Job
    steps:
      - uses: actions/checkout@v6
        with:
          ref: ${ github.head_ref }
          persist-credentials: true

      - name: example-step
        uses: nrkno/github-action-semantic-release@v2
        with:
          subcommand: <value>

      - name: Commit and push changes
        uses: stefanzweifel/git-auto-commit-action@v7
        with:
          commit_message: "docs(autodoc): update documentation"
```

### Full example usage

```yaml
---
name: Example Workflow using this Action
on:
  pull_request:

jobs:
  example-job:
    name: Example Job
    steps:
      - uses: actions/checkout@v6
        with:
          ref: ${ github.head_ref }
          persist-credentials: true

      - name: example-step
        uses: nrkno/github-action-semantic-release@v2
        with:
          subcommand: <value>
          token: ${{ github.token }} # Optional
          dry-run: false # Optional
          working-directory: . # Optional

      - name: Commit and push changes
        uses: stefanzweifel/git-auto-commit-action@v7
        with:
          commit_message: "docs(autodoc): update documentation"
```


<!-- autodoc end -->

---

## Configuration (`.semrelrc.yml`)

Place a `.semrelrc.yml` at your repo root to customise semrel behaviour.
All fields are optional — absent fields retain the defaults shown.

```yaml
# .semrelrc.yml
lint:
  rules:
    capital-first-letter: true  # fail UPPER descriptions
    require-scope: false

bump-rules:                     # which type triggers which bump
  breaking-change: major        # default — customise freely
  feat: minor
  fix: patch
  # chore: patch               # example: add patch bump for chore

release-branches: [main, master]  # branches that trigger a release

tag-prefix: "v"               # tag format: "v1.2.3" (set "" for bare)

commit-types:
  extra-types: []             # add custom types to the built-in 10
  allowed-types: []           # replace built-in set entirely (non-empty)

initial-version: "0.0.0"     # bootstrap base version (bump applied on top)
```

See [docs/configuration.md](/docs/configuration.md) for the full field reference.

---

## Documentation

Full documentation is in [`docs/`](/docs/index.md):

- [Architecture](/docs/architecture.md) — design decisions and package structure
- [Configuration](/docs/configuration.md) — environment variables and flags
- [API Reference](/docs/api-reference.md) — subcommand flags, exit codes, output fields
- [Playbook](/docs/playbook.md) — step-by-step runbooks

---

## Lockfile upkeep

`.github/workflows/actions.lock` is generated by the
[`gh actions-lock`](https://github.com/github/gh-actions-lock) extension. Step
`uses:` references keep full version tags (`actions/checkout@v7.0.1`); the
lockfile records the commit SHA each tag resolved to, including transitive
actions, and GitHub enforces those SHAs when the workflow runs. A moved or
re-pointed tag therefore cannot change what runs. Whenever a `uses:` reference
in `.github/workflows/` is added, removed or changed, update the lockfile in the
same PR:

```sh
gh extension install github/gh-actions-lock --pin v0.4.1  # once
gh actions-lock
```

Commit `actions.lock` together with any tag rewrite the extension makes.
Renovate regenerates the lockfile in its action-bump PRs; run the command above for hand edits.

Reusable workflow calls (`jobs.<job_id>.uses`) are not covered by the lockfile.
Keep them pinned to a full commit SHA with a `# vX.Y.Z` comment.

Steps in this repository's `action.yml` are not covered by the lockfile and stay pinned to a full commit SHA.
The `Actions lockfile` workflow runs `gh actions-lock --verify-local` on PRs that
touch `.github/workflows/` and fails if an action reference has no lockfile
entry. Do not edit the lockfile by hand.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
