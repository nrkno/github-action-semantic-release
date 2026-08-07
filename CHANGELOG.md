# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

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

[Unreleased]: https://github.com/nrkno/github-action-sematic-release/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/nrkno/github-action-sematic-release/releases/tag/v0.1.0

