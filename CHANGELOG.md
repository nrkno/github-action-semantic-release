# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- The container image is now published for `linux/amd64` and `linux/arm64`, so the action also runs on arm64 runners.

### Security

- The container runs unprivileged and read-only: it uses the runner's uid/gid (the image declares a non-root `USER`), drops all capabilities, sets `no-new-privileges`, and mounts only the workspace, `$GITHUB_OUTPUT` and the event payload (read-only). `RUNNER_TEMP` is no longer mounted. Workflows that relied on the container reading or writing other host paths must pass them through the workspace.
- The base image is now Alpine 3.24.4 (the previous Alpine 3.19 is end of life; fixes musl CVE-2026-40200 and busybox CVE-2024-58251), built with Go 1.26.11. `golang.org/x/net` is updated to v0.62.0 and `golang.org/x/crypto` to v0.59.0, fixing HIGH-severity CVEs in the shipped binary. Go modules, `go-git` (v5.19.3) and `go-github` (v93) are updated.
- `GITHUB_OUTPUT` values now use a random heredoc delimiter that cannot occur in the value, and invalid output keys are rejected. A fixed delimiter let a multi-line value (for example a commit message in release notes) end the block early and inject further outputs.

- Release workflow: the container image is built and signed before the tag exists, and the tagged commit carries the image digest in `action.yml`, so a commit-SHA pin selects exactly one signed image. The tagged commit is a child of the released `main` commit and is on no branch (GitHub shows a "does not belong to any branch" banner). The image `COMMIT` label is the `main` commit. The release job asserts the tag's commit is a direct child of the released commit and contains the digest exactly once.

### Changed

- `release` and `notify` now count the commits reachable from `HEAD` and not reachable from the baseline tag, whether or not the tag is an ancestor of `HEAD`. Previously a baseline tag that was not an ancestor of `HEAD` made every release count the full history. Tags with unrelated history behave as before. Releasing from several long-lived branches is not supported: commits released under a non-baseline tag are counted again.

### Fixed

- Network failures while calling the GitHub API now return an error. Previously the wrapper dereferenced the nil response that go-github returns when a request fails before a response arrives, and the process panicked.
- `release` now writes the `bump` output (`major`, `minor`, `patch` or `none`) that `action.yml` already declared but the binary never set. In `--dry-run` it also tells "nothing to release" apart from "would release", which the release workflow uses to avoid building an image for a first commit that does not warrant a release.
- `lint` on `push` events now validates the pushed commits from the GitHub event payload (`GITHUB_EVENT_PATH`) when available, preventing tagless repositories from linting their entire commit history on every branch push. Falls back to previous tag-based behaviour when the payload is unavailable. Explicit `--from-ref` overrides automatic detection.
- `lint` now actually lints `GITHUB_BASE_REF → HEAD` on `pull_request` events as documented. Previously the base ref was resolved but silently ignored, and the range fell back to latest-annotated-tag → HEAD (or full history when no annotated tag existed), causing PR lint failures for non-conventional commits already merged to the base branch.
- `lint` ranges now exclude all commits reachable from the range start, including via merge-commit parents. On GitHub's `refs/pull/N/merge` checkout, base-branch history reachable through the merge commit's second parent no longer leaks into the lint range.
- `--from-ref` is now honored for arbitrary refs (branches, `origin/<branch>`, tags, SHAs) instead of being overridden by the latest-tag lookup.

## [0.1.0] - 2026-06-29

### Added

- `semrel` Go CLI binary with four subcommands: `lint`, `release`, `notify`, `notes`
- `lint` — validates conventional commits in the range relevant to the current GitHub Actions event (PR base → HEAD, or previous tag → HEAD on push)
- `release` — computes next semver from conventional commit bump signals, creates an annotated git tag, pushes it, and creates a GitHub Release; idempotency ladder guards against double-releases
- `notify` — posts a deduplicated `<!-- semrel-notify:<version> -->` comment on the merged PR after a successful release
- `notes` — generates Markdown release notes from conventional commits and linked PRs, suitable for step summary output
- Distroless container image (`gcr.io/distroless/static-debian12:nonroot`) published to GHCR with cosign keyless signing
- Multi-architecture image build (linux/amd64, linux/arm64)
- CI workflow: unit tests, race detector, integration tests, golangci-lint, conventional commit lint
- Release workflow: build-from-source (no bootstrap dependency on the container), publish image, notify

### Security

- Replaces the compromised third-party `codfish/semantic-release` GitHub Action
- Uses only `GITHUB_TOKEN`; no external secrets required
- Container signed with cosign keyless (Sigstore) for supply-chain attestation

[Unreleased]: https://github.com/nrkno/github-action-semantic-release/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/nrkno/github-action-semantic-release/releases/tag/v0.1.0

