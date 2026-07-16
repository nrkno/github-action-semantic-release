---
type: Log
title: semrel documentation log
description: Dated change log for the semrel docs/ directory, recording every document creation and significant update.
tags: [log, changelog, documentation]
timestamp: 2026-07-16
---

# Documentation log

### 2026-07-16

- **Update** `docs/configuration.md` — replaced `--dry-run` description with updated preview-mode semantics: skips branch guard for PR branches, computes projected next version, writes all output fields, performs zero write operations

### 2026-07-01

- **Update** `docs/architecture.md` — reconciled with post-security-audit reality (Wave 1 fixes): removed all stale distroless/nonroot container claims and replaced with an accurate description of the `alpine:3.19` (digest-pinned) base image running as root, with rationale (workspace ownership by the runner's root user) and the two mitigating controls (cosign verification enforced before every run, reviewed/CI-built image pipeline); removed the Docker container-action `image: docker://...:v1.2.3` snippet and replaced with the composite action mechanism (`runs.using: composite`, self-referential `ghcr.io/${{ github.action_repository }}:${{ github.action_ref }}` image tag, no chore-commit PR needed per release); documented that `cosign verify` is now enforced at run time in the composite action's `verify` step, not merely a documented recommendation; added F4 residual-risk disclosure under "What these layers do NOT protect against" — a cosign-verified image still runs as root with RW access to `$GITHUB_OUTPUT`/`$GITHUB_ENV`/`$GITHUB_WORKSPACE`, so consumers should avoid unsafe interpolation of action outputs into later `run:` steps; updated front-matter `description`/`tags` (dropped `distroless`, added `alpine`) and `timestamp` to 2026-07-01

### 2026-06-30

- **Update** `docs/configuration.md` — renamed semrel-specific section to cover all subcommands; added `SEMREL_LOG_LEVEL` row (scope=all subcommands) documenting DEBUG/INFO/WARN/ERROR log verbosity control via environment variable
- **Update** `docs/api-reference.md` — updated `semrel lint` exit code 1 to describe structured `slog.Error` per-commit violations with `sha`/`rule`/`message`/`example` fields; added `"no release: no bump-worthy commits"` (INFO) and `"proceeding with full release"` (INFO) rows to `semrel release` log output table; added shallow-clone detection note under `semrel release` exit codes documenting immediate code-2 exit with `level=ERROR` and `fix` field

### 2026-06-30

- **Update** `README.md` — replaced digest-pinning paragraph in Security section with cosign keyless verification as the supply-chain integrity mechanism; updated recommendation to pin to release tag or commit SHA rather than `@main`
- **Update** `docs/architecture.md` — replaced "Two-layer pinning" section with updated Security Model explaining why digest pinning was dropped (GITHUB_TOKEN cannot push to branch-protected main); documented cosign keyless signatures as the container-layer verification mechanism; updated two-layer table to reflect tag + cosign instead of tag + digest; timestamp updated to 2026-06-30
- **Update** `action.yml` — image reference updated from `v0.1.2@sha256:…` to `v0.3.0` (mutable version tag, no digest suffix)
- **Update** `.github/workflows/release.yml` — sed replacement in `Update action.yml image reference` step drops `@${DIGEST}` suffix so future releases write `:${TAG}` only

### 2026-06-30

- **Update** `docs/api-reference.md` — rewrote `semrel notify` section: removed all references to `pull_request` event, `SEMREL_RELEASED`, and `GITHUB_REF` PR parsing; replaced with release-event trigger model; documented `SEMREL_TAG` (required), `SEMREL_RELEASE_URL` (optional), `SEMREL_VERSION` (optional); documented commit-range fan-out to PRs, idempotent comment marker `<!-- semrel-notify:<tag> -->`; updated example to show `notify.yml` as a separate workflow (not a job in `release.yml`); separated permissions blocks for release vs notify workflows
- **Update** `docs/configuration.md` — removed `SEMREL_RELEASED` row from notify env vars table; added `SEMREL_RELEASE_URL` row (scope=notify, auto-constructed if absent); updated `SEMREL_TAG` description to "Required for semrel notify"; updated section heading to reflect release-event payload sourcing
- **Update** `docs/playbook.md` — updated first-release runbook to list `notify.yml` as a third workflow file (separate from `release.yml`), triggered by `on: release: types: [published]`; updated verify steps to show notify triggering from GitHub Release publication; updated skip-release runbook; added `fetch-tags: true` note for shallow clone fix

### 2026-06-29

- **Update** `README.md` — added Configuration (.semrelrc.yml) section with all 5 new fields
- **Update** `docs/playbook.md` — added 5 runbooks for new config options

### 2026-06-29

- **Update** `docs/configuration.md` — added 5 new .semrelrc.yml fields: bump-rules, release-branches, tag-prefix, commit-types, initial-version
- **Update** `docs/api-reference.md` — noted new config fields in lint and release sections
- **Update** `docs/api-reference.md` — added "Log output" subsection to `semrel release` documenting the 8 structured slog.Info lines emitted during a release (commits in release, bump detected, PRs in release, release triggered by, created annotated tag, pushed tag, created GitHub release, bootstrap case)

### 2026-06-29

- **Update** `docs/configuration.md` — added `.semrelrc.yml` lint configuration file section (capital-first-letter and require-scope rules, absent/malformed-file behaviour, working-directory note)
- **Update** `docs/api-reference.md` — noted `.semrelrc.yml` lookup in lint subcommand description; updated exit code 1 to cover malformed config file
- **Update** `docs/architecture.md` — added Security Model section covering two-layer pinning (git SHA + container digest), what each layer protects against, what is out of scope, and cosign verification recommendation
- **Update** `docs/playbook.md` — added "How to pin securely" runbook: find commit SHA on releases page, use full SHA as workflow ref, optional cosign verification
- **Update** `README.md` — added Security section (before Quick Start) with SHA-pinning warning, secure vs less-secure usage examples, and explanation of dual-layer supply-chain protection
- **Update** `.github/workflows/release.yml` — emit `release_sha` job output from `publish-image` (captures `git rev-parse HEAD` after retag); update release job summary step to print commit SHA alongside release notes

### 2026-06-29

- **Creation** `action.yml` — GitHub Action manifest: 4 inputs (subcommand, token, dry-run, working-directory), 9 outputs (released, version, tag, major_version, minor_version, patch_version, bump, notes, sha), docker image reference with tag+digest placeholder
- **Creation** `entrypoint.sh` — shell entry point that maps `INPUT_*` environment variables to semrel CLI arguments and exports `GITHUB_TOKEN`; copied into the Docker image and used as `ENTRYPOINT`
- **Update** `docs/configuration.md` — added `INPUT_*` subsection documenting the four GitHub Action input variables (`INPUT_SUBCOMMAND`, `INPUT_TOKEN`, `INPUT_DRY_RUN`, `INPUT_WORKING_DIRECTORY`) and their mapping to CLI arguments
- **Update** `docs/api-reference.md` — added "Using as a GitHub Action" section with inputs table, outputs table, required permissions, and a complete lint+release+notify workflow example

### 2026-06-29

- **Creation** `docs/architecture.md` — initial architecture documentation covering supply-chain context, package structure, go-git rationale, distroless container, idempotency ladder, SHA comparison, and shallow clone requirement
- **Creation** `docs/configuration.md` — complete reference for all environment variables consumed by semrel, subcommand flags (lint --from-ref/--to-ref, release --dry-run), and GITHUB_OUTPUT fields (released, version, tag, major, minor, patch)
- **Creation** `docs/api-reference.md` — subcommand synopsis, flags, exit codes, stdout/stderr behaviour, and GITHUB_OUTPUT fields for all four subcommands (lint, release, notify, notes)
- **Creation** `docs/playbook.md` — step-by-step runbooks: first release in a new repo, shallow clone fix, conflict resolution, skipping a release, bootstrapping without prior tags, debugging lint failures, verifying cosign signatures
- **Creation** `docs/index.md` — table of contents listing all docs with bundle-relative links
- **Creation** `docs/log.md` — this log file, documenting the documentation pass
- **Creation** `README.md` — project overview with badges, quick-start, full workflow snippet, permissions table, subcommand table, fetch-depth warning
- **Creation** `CHANGELOG.md` — Keep a Changelog format with initial [0.1.0] entry
- **Creation** `LICENSE` — MIT license, copyright NRK 2026
- **Creation** `CONTRIBUTING.md` — conventional commit requirement, PR process, local dev setup, project structure overview
- **Creation** `SECURITY.md` — vulnerability reporting policy, scope, disclosure policy, cosign verification instructions
