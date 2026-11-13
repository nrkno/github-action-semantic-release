---
type: Playbook
title: semrel playbook
description: Step-by-step runbooks for common semrel operations — first release, debugging failures, handling conflicts, bootstrapping, and skipping a release.
tags: [playbook, runbook, first-release, debugging, bootstrap, troubleshooting]
timestamp: 2026-11-13
---

# Playbook

## Runbook: first release in a new repo

**Situation:** A repository has never had a semver tag and you want semrel to manage
future releases.

**Steps:**

1. **Add the release workflow.** Copy the full workflow from the [README](/README.md)
   to `.github/workflows/release.yml`. Ensure `fetch-depth: 0` is set on the checkout step.

2. **Add the CI workflow.** Copy `.github/workflows/ci.yml` to lint commits on every PR.

3. **Add the notify workflow.** Copy `.github/workflows/notify.yml` to
   `.github/workflows/notify.yml`. This workflow is triggered by
   `on: release: types: [published]` — a separate workflow, not a job inside
   `release.yml`. It requires `fetch-depth: 0` and `fetch-tags: true` on the
   checkout step so that `semrel notify` can find the previous annotated tag.

3. **Make sure all existing commits are conventional.** Run lint locally:

   ```bash
   go build -o semrel ./cmd/semrel
   ./semrel lint
   ```

   Fix any violations before pushing. The first release reads all commits from the
   beginning of history when there is no prior tag.

4. **Push a releasable commit to `main`.** A `feat` or `fix` commit triggers a release.
   A `chore`-only push produces `released=false` and no tag.

5. **Verify.** The release workflow run should:
   - Exit 0 on the `./semrel release` step
   - Show `released=true` in step outputs
   - Create tag `v0.0.1` (bootstrap always starts at `0.0.1`)
   - Create a GitHub Release

6. **Verify notify.** The GitHub Release publication triggers `notify.yml`:
   - Finds all commits between root and `v0.0.1`
   - Posts `<!-- semrel-notify:v0.0.1 -->` comments on all associated PRs

---

## How to run semrel in a new repo

1. Copy all three workflow files (`ci.yml`, `release.yml`, `notify.yml`) from this repository.
2. The repository needs no special configuration — `GITHUB_TOKEN` is automatic.
3. Grant the required permissions (see [README — Required permissions](/README.md)):
   - `contents: write`
   - `packages: write` (only if publishing a container image)
   - `id-token: write` (only if signing with cosign)
4. Push a conventional commit to `main`.

---

## What to do when release fails: conflict (exit 1)

**Symptom:** `./semrel release` exits with code 1 and a log message like:

```
tag v1.2.3 exists but points to different commit: abc1234 vs def5678
```

**Cause:** Two workflow runs computed the same next version but tagged different
commits. This can happen if two `feat` commits land in quick succession and two
workflow runs start before either creates the tag.

**Resolution:**

1. Identify which tag is correct. Check `git log --oneline v1.2.3` and the GitHub
   Release page.
2. If the existing tag is wrong (points to the wrong commit), delete it:

   ```bash
   git push origin :refs/tags/v1.2.3   # delete remote tag
   git tag -d v1.2.3                   # delete local tag
   ```

3. Re-run the failed workflow. It will proceed through the full flow and create the
   tag correctly.

4. If both tags are wrong, delete both and re-run from the later commit.

---

## What to do when release fails: shallow clone (exit 2)

**Symptom:** `./semrel release` exits with code 2 and a message like:

```
level=ERROR msg="repository is shallow"
```

**Cause:** The checkout step used the default `fetch-depth: 1` (or omitted
`fetch-depth`). semrel requires the full history.

**Fix:** Add `fetch-depth: 0` to the `actions/checkout` step:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0
```

This applies to every job that runs a semrel subcommand (`release`, `lint`, `notes`, `notify`).
The `notify.yml` workflow additionally requires `fetch-tags: true` so that
`semrel notify` can locate the previous annotated tag.

---

## How to skip a release

**Situation:** You want to merge a commit to `main` without triggering a release
(e.g., a documentation update, CI fix, or dependency bump that doesn't warrant a
version bump).

**Solution:** Use a non-releasable commit type. The following types produce
`released=false`:

- `docs`
- `style`
- `test`
- `build`
- `ci`
- `chore`

Example:

```
chore: update golangci-lint to v1.59
```

The release workflow still runs, but `released=false` means no tag is created, no
GitHub Release is published, and the `publish-image` job is skipped. Since no
Release is published, the `notify.yml` workflow is not triggered either.

---

## How to bootstrap (no prior tags)

When there are no annotated tags in the repository, semrel treats the entire commit
history as the range and starts versioning from `v0.0.1`.

- A `feat` or `fix` commit anywhere in history produces `v0.0.1`
- A `BREAKING CHANGE` commit still starts at `v0.0.1` (no `v1.0.0` jump on bootstrap)

If you want to start at a specific version (e.g., `v2.0.0`), create the tag manually
before running semrel:

```bash
git tag -a v1.9.9 -m "pre-semrel baseline" <initial-commit-sha>
git push origin v1.9.9
```

semrel will then compute the next version relative to `v1.9.9`.

---

## How to debug lint failures

**Symptom:** The `lint-commits` CI job fails with:

```
commit abc1234: missing type
  some unclear message
  example: fix: correct handling of empty input
```

**Steps:**

1. **Run lint locally** with the same range as CI:

   ```bash
   go build -o semrel ./cmd/semrel

   # On a PR branch — same as the CI job
   ./semrel lint --from-ref origin/main --to-ref HEAD
   ```

2. **Identify the offending commit.** The short SHA is printed with each violation.

3. **Amend or reword** the commit:

   ```bash
   # If it's the most recent commit
   git commit --amend

   # If it's further back, use interactive rebase
   git rebase -i origin/main
   # Change "pick" to "reword" for the offending commit
   ```

4. **Force-push** the branch (PRs only — never force-push `main`).

5. **Verify** by re-running lint locally before pushing.

**Common violations:**

| Message | Fix |
| ------- | --- |
| `missing type` | Add a type prefix: `fix:`, `feat:`, `chore:`, etc. |
| `missing description` | Add a description after the colon: `feat: add retry logic` |
| `subject too short` | Description must be at least 3 characters |
| `type not allowed` | Use one of: `feat fix docs style refactor perf test build ci chore revert` |

---

## How to restrict releases to a single branch

By default, `semrel release` runs on both `main` and `master`.
To restrict it to `main` only:

```yaml
# .semrelrc.yml
release-branches: [main]
```

Glob patterns are supported (`releases/*` matches `releases/v2`).
Note: `*` does not cross `/` — `releases/*` will NOT match `releases/team/v2`.

---

## How to bootstrap at a non-zero version

When no annotated tags exist, semrel starts at `0.0.0` and applies the
detected bump. To start at a different baseline:

```yaml
# .semrelrc.yml
initial-version: "1.0.0"
```

With this config, the first `fix:` commit produces `v1.0.1`, and the first
`feat:` commit produces `v1.1.0`.

---

## How to add or restrict valid commit types

To add custom types on top of the built-in 10:

```yaml
# .semrelrc.yml
commit-types:
  extra-types: [deps, security]
```

To restrict lint to an exact subset:

```yaml
commit-types:
  allowed-types: [feat, fix, docs]
```

---

## How to customise which commit types trigger a version bump

Override any type's bump level, or add bump rules for types that default to none:

```yaml
# .semrelrc.yml
bump-rules:
  chore: patch    # add patch bump for chore
  fix: none       # suppress fix bumps
```

To freeze all bumps (no releases on any commit type):

```yaml
bump-rules:
  breaking-change: none
  feat: none
  fix: none
```

---

## How to use bare version tags

Set `tag-prefix: ""` to produce tags like `1.2.3` instead of `v1.2.3`.

```yaml
# .semrelrc.yml
tag-prefix: ""
```

> ⚠️ Only set this on new repositories. Changing the prefix on a repo with
> existing `v`-prefixed tags will cause semrel to stop finding them.

---

## How to verify cosign signature

After a release, verify the container image signature:

```bash
# Read the digest from action.yml at the release commit you pinned
IMAGE=$(grep -o 'ghcr.io/[^"]*@sha256:[0-9a-f]\{64\}' action.yml | head -1)
cosign verify "$IMAGE" \
  --certificate-identity="https://github.com/nrkno/github-action-semantic-release/.github/workflows/release.yml@refs/heads/main" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com"
```

A successful verification prints the signing certificate and bundle to stdout.

---

## How to pin securely

**Situation:** You want to use this action in a workflow with an immutable reference
so your pipeline cannot be silently altered by a tag being force-pushed.

**Why this matters:** Git tags are mutable. Anyone with push access to this
repository can run `git push --force origin v1.2.3` to point the tag at a different
commit. Pinning by commit SHA is immune to this — the SHA is a cryptographic hash
of the commit content and cannot be redirected.

**Steps:**

1. **Find the release on the releases page.**
   Go to [github.com/nrkno/github-action-semantic-release/releases](https://github.com/nrkno/github-action-semantic-release/releases)
   and open the release you want to use.

2. **Copy the commit SHA.**
   Each release workflow prints the commit SHA in the workflow summary
   (visible by clicking the release workflow run linked from the release page).
   The SHA is also the "full SHA" shown next to the tag on the releases page.

3. **Use the SHA as the ref in your workflow:**

   ```yaml
   # Pin to a specific commit SHA (see Releases for the SHA of each release)
   - uses: nrkno/github-action-semantic-release@<COMMIT_SHA>
     with:
       subcommand: release
       token: ${{ secrets.GITHUB_TOKEN }}
   ```

   Replace `<COMMIT_SHA>` with the full 40-character SHA, e.g.:

   ```yaml
   - uses: nrkno/github-action-semantic-release@a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2
   ```

4. **(Optional) Verify the cosign signature** to confirm the image at that SHA
   was built by NRK's CI pipeline:

   ```bash
   IMAGE=$(grep -o 'ghcr.io/[^"]*@sha256:[0-9a-f]\{64\}' action.yml | head -1)
   cosign verify "$IMAGE" \
     --certificate-identity="https://github.com/nrkno/github-action-semantic-release/.github/workflows/release.yml@refs/heads/main" \
     --certificate-oidc-issuer="https://token.actions.githubusercontent.com"
   ```

**Result:** Your workflow references an immutable git object (commit SHA) whose
`action.yml` contains an immutable container reference (image digest). Together,
these two layers give full supply-chain protection at both the git and registry
layers.
