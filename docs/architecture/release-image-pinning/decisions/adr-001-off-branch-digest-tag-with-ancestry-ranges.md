---
type: Architecture
title: "ADR-001: Ship the image digest on an off-branch release commit and compute ranges by ancestry difference"
description: Keeps the digest-pinned action.yml on a tagged child commit of main, and replaces semrel's all-history fallback for unreachable tags with the set difference reachable(HEAD) minus reachable(tag), in both release/notes and notify ranges.
tags: [adr, release, supply-chain, image-digest, cosign, commit-ranges, git-tags]
timestamp: 2026-11-13
---

**Status:** accepted with the critic's amendments (2026-11-13); implemented on branch `security/sweep-2026-11` (git.go, release.yml, tests), not yet released. Supersedes the "off-branch selected tag keeps all HEAD history" behavior recorded in [commit-ranges ADR-001](/docs/architecture/commit-ranges/decisions/adr-001-pruned-commit-range-enumeration.md).

## Summary

A consumer who pins this action to a commit SHA must get exactly one signed container
image. The release workflow therefore builds and signs the image first, then writes its
digest into `action.yml` in a new commit, and tags that commit. That commit is a child of
the `main` commit that was released, but it is not on `main`.

semrel currently treats a release tag that is not an ancestor of HEAD as "no usable
baseline" and counts **all** HEAD history (`internal/git/git.go:261-293`). With
off-branch release commits, that is every release, so every later push to `main`
re-releases all history (reviewer finding C1).

**Decision:** keep the off-branch digest commit and change semrel's commit-range rule.
The commits in a release are those reachable from HEAD but not reachable from the
baseline tag's commit. When the tag is an ancestor of HEAD this is today's behavior.
When it is not, it is the commits since the shared history (the merge base), instead of
all history. The same rule replaces the first-parent-by-time walk in
`ListCommitsBetweenTags` used by `notify`.

**Normal example.** `main` is at `M1`. The release job creates `C1` (parent `M1`, only
change: the digest in `action.yml`) and tags it `v1.4.0`. Later `main` gets `M2 fix: …`.
On that push semrel's baseline is `v1.4.0` → `C1`. reachable(`M2`) minus reachable(`C1`)
is `{M2}`, because `M1` and everything before it is an ancestor of `C1`. The release is
`v1.4.1` with notes for `M2` only. The release job creates `C2` (parent `M2`) and tags it.
`notify` for `v1.4.1` lists reachable(`C2`) minus reachable(`C1`) = `{C2, M2}`.

**Merge constraint.** The `internal/git/git.go` range change and the off-branch tagging
in `release.yml` must land in the same merge. Off-branch tagging with the old range code
re-releases all history on every push. Both are on the same branch, so this holds.

**Limits.** This does not change which tag is selected as the baseline (still highest
semver across matching tags). It does not fix the pre-existing gap that a run interrupted
after the tag push but before GitHub release creation is not completed automatically by
`semrel release`; the workflow detects it and fails loudly (see release.yml changes).

## Satisfies

No formal requirement documents exist for this topic. Traced to the stated goals:

| Goal / constraint | Disposition |
| --- | --- |
| A1 (security report): a commit-SHA pin must imply one exact, signed image | Satisfied: the tagged commit's `action.yml` contains `image@sha256:<digest>`; `action.yml` verifies and runs that digest only. |
| C1 (review): later pushes must not re-release, notes must not include all history, notify ranges and reruns must work | Satisfied by the ancestry-difference rule plus a deterministic digest commit. |
| Public inputs and outputs of the action unchanged | Satisfied: no change to `inputs:`/`outputs:`. |
| Least dependence on org settings | Satisfied: needs no ruleset bypass, GitHub App or branch push rights. |
| Version baseline selection (architecture ADR-001) | Unchanged: semver-max across tags. |
| Release range when the tag is an ancestor of HEAD (commit-ranges ADR-001) | Unchanged: identical result. |
| Unreachable tag (commit-ranges ADR-001, "existing fallback preserved by scope") | Changed by this decision; see Consequences. |

## Alternatives

### A. Bot commits the digest to `main` and tags that commit (reviewer option a)

The tag is then an ancestor of HEAD and the current range code works.

- Needs a GitHub App (or deploy key) that bypasses the `main` ruleset, because pushes
  require a pull request. That is a privileged, always-on push path to `main`, which is
  the thing the security sweep is reducing. A `GITHUB_TOKEN` push cannot be a bypass
  actor and would not retrigger workflows anyway.
- The push races other merges: if `main` moved after the plan job, the push is rejected
  and must be rebased, which changes the commit that was planned.
- After the first release, every unreleased `main` commit carries the *previous*
  release's digest. A consumer who pins an unreleased SHA then silently runs an older
  image instead of hitting today's "no image digest" error. That weakens "pin implies
  the image built from this code".
- Adds a bot commit to `main` per release.

Loses on org-setting dependence, privilege and pin semantics.

### B. Off-branch digest commit + ancestry-difference ranges in semrel (chosen; reviewer option b)

- No org settings, no extra credentials, no writes to `main`.
- `main` keeps the all-zero digest, so unreleased SHAs fail closed.
- The range rule is the same set-difference contract `ListCommitsSinceRef` already uses
  for PR lint, and reuses `ancestorSet`/`commitsFromHead`. It removes code (the
  reachability check) rather than adding machinery.
- Consumer-visible change: see Consequences.

### C. Leave `action.yml` unchanged; resolve the digest at run time from a signed attestation

For example, look up the image whose provenance or cosign certificate names the pinned
commit SHA.

- The pin no longer implies *one* image. Any rerun of the image job signs another valid
  image for the same commit, and the action cannot tell which one is "the" release
  without an extra authority (registry tag, release asset), which is mutable or
  needs more trust anchors.
- Adds a registry/attestation lookup and network failure mode to every consumer run.
- `uses: …@<sha>` of an unreleased commit can still resolve to an image if one was
  signed for it, blurring the release boundary.

Loses on the core constraint.

### D. Tag `main` itself and keep the digest elsewhere (release asset, separate file)

Same objection as C: the pinned commit does not contain the digest, so the pin does not
determine the image.

## Decision details

### 1. semrel range rule (`internal/git/git.go`)

`ListCommitsSinceTag(tag)`:

- `tag == nil`: unchanged (all HEAD history, bootstrap).
- Otherwise: resolve the tag's target commit exactly as today (same errors for a missing
  or non-commit target), then return `commitsFromHead(HEAD, ancestorSet(target))`.
- Remove the `headAncestors` computation and the reachability branch. Update the doc
  comment: "commits reachable from HEAD and not reachable from the tag's target commit".

`ListCommitsBetweenTags(from, to)`:

- `from == nil`: unchanged result (all history reachable from `to`).
- Otherwise: `commitsFromHead(to.Target, ancestorSet(from.Target))`. This replaces the
  committer-time walk that stops at the `from` hash, which never stops when `from` is
  not on the walked path and also drops merged commits older than `from`.
- Generalise `commitsFromHead`'s parameter name (it already takes an arbitrary start
  hash); keep its "read HEAD commit" error text accurate (e.g. "failed to read start
  commit").

Ordering: both functions keep newest-first by committer time, as `commitsFromHead`
already returns. No interface change to `GitClient`; `cmd/semrel/main.go` lazy wrappers
are unaffected.

Invariants:

- I1. For any tag target T that is an ancestor of HEAD, the result is identical to the
  current implementation.
- I2. No commit reachable from the baseline tag's commit is ever counted again.
- I3. A commit that is reachable from HEAD and not from the tag is always counted,
  regardless of timestamps.

### 2. Release workflow (`.github/workflows/release.yml`)

Keep the plan → image → release structure and the cosign identity. In the release job:

- **Deterministic digest commit.** Create the commit with fixed identity and dates so the
  same `(github.sha, digest)` always yields the same commit SHA:
  `GIT_AUTHOR_DATE` and `GIT_COMMITTER_DATE` set to `git show -s --format=%cI "$GITHUB_SHA"`,
  identity as today, message `chore(release): pin image digest for ${TAG}` (a `chore`
  type, so it never produces a bump on its own). A rerun of the release job with the same
  image output then reproduces the already-tagged commit.
- **Rerun-tolerant, content-checking check step.** Replace "Check released tag matches the
  signed image" with: pass the first condition if `released == true` and
  `tag == plan.tag`; otherwise pass it only if `refs/tags/$EXPECTED` (peeled with
  `^{commit}`) equals `git rev-parse HEAD` **and** a GitHub release for `$EXPECTED` exists
  (`gh release view "$EXPECTED"` with `GH_TOKEN`). If the tag is at HEAD but no release
  exists, fail with an error that tells the maintainer to create the release for that
  tag. Any other combination fails as today. Validate `$EXPECTED` with the same regex as
  the plan job before using it. On both paths, then assert about the tag's commit
  `T = refs/tags/$EXPECTED^{commit}`:
  - `git show "$T:action.yml"` contains `@${DIGEST}` exactly once (the tag carries the
    signed image from this run); and
  - the parent of `T` equals `$GITHUB_SHA` (the tag is a direct child of the released
    `main` commit).
- **Plan job, decide step.** If `refs/tags/$TAG` exists but `gh release view "$TAG"` fails,
  fail with recovery guidance instead of reporting `release=false`: the tag was pushed but
  the release was not created (or the lookup failed); create the release for the existing
  tag, or investigate the tag, then rerun. The job already has `contents: read`, which
  `gh release view` needs.
- Keep the existing header comment but state that the tagged commit is a child of the
  released `main` commit and is on no branch, and that semrel's ancestry-difference rule
  (this ADR) is what makes that safe.

No change to the plan or image jobs.

### 3. `action.yml`

No change. It already verifies and runs `image@sha256:<digest>`, keeps the all-zero
sentinel on `main`, and pins the identity to `release.yml@refs/heads/main`. Inputs and
outputs stay identical.

## Consequences

- **Consumer behavior change (semrel binary).** Repositories whose highest-semver tag is
  not an ancestor of HEAD but shares history with it now release only commits since the
  shared history, not all history. The old behavior re-counted already released commits
  and could re-release or inflate the bump. Tags with unrelated history give the same
  result as before (all HEAD history). Ship this as a `fix(git):` commit and call it out
  in the release notes.
- **Maintenance branches are unsupported.** Only the baseline tag's ancestry is excluded.
  Commits that were released under a different, non-baseline tag and are not reachable
  from the baseline tag are counted again. Releasing from several long-lived branches in
  one repository is therefore not supported by this rule.
- **`notify` ordering limitation (known, not fixed here).** `notify` picks the previous
  tag with `FindPreviousAnnotatedTag`, which orders annotated tags by the author date of
  their target commit, not by semver or ancestry. The digest commit takes its dates from
  the released `main` commit, so ordering follows `main`, but clock skew or equal dates
  can still select a different previous tag. The range itself is correct for whichever
  previous tag is chosen. Changing the selection is out of scope.
- **Consumer-visible banner.** GitHub shows "This commit does not belong to any branch on
  this repository, and may belong to a fork outside of the repository" on the digest
  commit's page. This is expected for an off-branch release commit.
- **Image `COMMIT` label.** The image's build `COMMIT` and its
  `org.opencontainers.image.revision` label are the `main` commit (`github.sha`), the
  parent of the tagged digest commit. The tagged commit differs from it only in
  `action.yml`.
- The open item "off-branch release baseline policy remains unresolved" in
  `docs/log.md` is resolved by this ADR.
- An interrupted run (tag pushed, GitHub release not created) is still not repaired by
  `semrel release`; the workflow now reports it instead of silently passing or creating a
  different version.

## Org and repository settings

None are required for this design to work. Recommended: a tag ruleset on `v*` that blocks
deletion and update (non-fast-forward) of release tags, because moving or deleting a
release tag breaks the one-tag-one-image guarantee; and a read-only default
`GITHUB_TOKEN`. Restricting tag *creation* is of limited value: rulesets restrict by
actor, not by workflow path, and the release job pushes the tag with `GITHUB_TOKEN`, so a
creation restriction would need a bypass that any workflow in the repository can use.

## Verification

Required evidence before relying on the design:

1. `internal/git/git_test.go`:
   All fixtures use distinct, explicit commit dates (never `time.Now()` ties), and
   ranges are asserted as sets, so the tests do not depend on ordering.
   - Replace `TestListCommitsSinceTag_OffBranchKeepsAllHeadHistory` with
     `TestListCommitsSinceTag_OffBranchTagExcludesSharedHistory`: `base` → `M1`
     (HEAD branch); `C1` = child of `M1` tagged `v1.0.0`; `M2` = child of `M1` on the
     branch. Expect exactly `{M2}`.
   - Add `TestListCommitsSinceTag_UnrelatedTagKeepsAllHeadHistory`: tag on an orphan
     root. Expect all HEAD commits (documents the unchanged case).
   - Add `TestListCommitsBetweenTags_OffBranchTags`: `M1`→`C1`(v1.0.0), `M2`→`C2`(v1.1.0)
     where `C2`'s parent is `M2`, `M2`'s parent is `M1`. Expect exactly `{C2, M2}`.
   - Add `TestListCommitsBetweenTags_MergeOlderThanFrom`: a feature commit authored
     before `from` and merged after it is included (I3).
   - Existing `TestListCommitsBetweenTags*`, `TestListCommitsSinceTag_*` and
     `TestCommitRangesReadOnlyHeadersForExcludedHistory` must still pass unchanged (I1).
2. `internal/cli/cli_test.go`, real on-disk repository (pattern of
   `TestReleaseWithProductionGitRepository`):
   `TestReleaseWithOffBranchDigestTagsProductionGitRepository`. Build
   `M0 feat!: historical` → `M1 feat: a` on the branch, then `C1 chore(release): pin
   image digest` (parent `M1`, not on the branch, annotated tag `v1.0.0`).
   - Rerun safety: with HEAD detached at `C1`, `--dry-run` reports `released=false`
     and creates no tag.
   - Append `M2 fix: x` → `M3 docs: y` on the branch. With HEAD at `M3`: `version=1.0.1`
     (not `2.0.0`, which the old fallback produces from `M0`), and the commits counted
     are `M2`, `M3` only.
   - Create `C2` (parent `M3`) tagged `v1.0.1`, append `M4 docs: z`, and assert
     `released=false` (no re-release; the old fallback releases again here).
   This test must fail on the current code and pass after the change.
3. `cmdNotify` path: a test that runs notify for `v1.0.1` in the repository above and
   asserts the commit list is `{C2, M3, M2}`.
4. `go test -race ./...` and `go vet ./...` pass; `actionlint` passes on `release.yml`.
5. Not provable offline: GitHub-side behavior (the plan/check steps against a real
   release, tag fetch behavior, the branch banner, ruleset effects). First real release
   is the proof; see the handback list.
6. Revisit trigger: if a consumer reports a release that skipped commits after this
   change, inspect whether their baseline tag was off-branch and whether the merge-base
   semantics matched their intent.
