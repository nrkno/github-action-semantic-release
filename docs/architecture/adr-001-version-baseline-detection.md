---
type: Architecture
title: "ADR-001: Version baseline detection strategy"
description: Decides how semrel selects the "last release" version that anchors next-version computation — evaluating semver-max across all tags, GitHub Releases API, and a hybrid annotated-preference approach.
tags: [adr, version-detection, git-tags, annotated-tags, semver, migration, codfish]
timestamp: 2026-08-19
---

**Status:** proposed

## Problem

`FindLatestAnnotatedTag()` (source: `internal/git/git.go:99–153`) applies a hard binary
fork when selecting the baseline version:

- **Tier 1** fires when **any** annotated tag exists. It collects only annotated tags,
  sorted by target-commit `Author.When` (descending), and returns the newest. It is
  completely blind to lightweight tags.
- **Tier 2** fires only when **zero** annotated tags exist. It collects lightweight tags,
  sorted by target-commit `Committer.When`, and returns the newest.

The old `codfish/semantic-release` workflow created annotated `vMAJOR.MINOR` alias tags
alongside every patch release (e.g. `v3.6` annotated, pointing to the same commit as
`v3.6.2`). The new semrel workflow creates annotated tags only for full `vMAJOR.MINOR.PATCH`
releases. Any repo migrated from codfish therefore has a permanent cohort of old annotated
alias tags from pre-migration history and zero annotated tags from post-migration releases.

**Consequence:** Tier 1 always fires (at least one annotated alias exists). It finds the
newest annotated tag — the last codfish-created alias (e.g. `v3.6`) — and uses that as the
baseline forever. All post-migration releases, whether created by semrel or by a human via
the GitHub Releases UI, produce only lightweight `vMAJOR.MINOR.PATCH` tags. Those tags are
permanently invisible to Tier 1.

**Observed failure (ground truth):** In `nrkno/iac-terraform-azure-postgres-flexible`, all
14 annotated tags are `vMAJOR.MINOR` aliases from `v1.1` through `v3.6` (pre-migration). All
32 post-migration tags — including the human-created `v4.0.0` (commit `cf7e362f`,
2026-08-06) — are lightweight. Tier 1 returns `v3.6`. semrel computes `v3.7.0`, finds it
already exists, logs `"release already exists"`, and exits 0. No release is ever created
post-migration. The `v4.0.0` tag is reachable from HEAD (full-history checkout confirmed;
`fetch-depth: 0` verified in the shared workflow at `nrkno/github-workflow-semantic-release@v5.1.4`).

**Scope:** This is the primary migration scenario the tool was built to handle. It affects
every repo previously on `codfish/semantic-release` that has been migrated to semrel.

**The same bug affects `cmdLint` (lines 122, 138) and `cmdNotes` (line 761):** a lint run
scans from the wrong baseline, and notes generation covers the wrong commit range.

**Root cause in code:**

```go
// internal/git/git.go:106–112
err = tags.ForEach(func(ref *plumbing.Reference) error {
    obj, err := r.raw.TagObject(ref.Hash())
    if err != nil {
        // Not an annotated tag (lightweight tag), skip   ← Tier 1 blind spot
        return nil
    }
    // …
})

// internal/git/git.go:135–152
if len(annotatedTags) > 0 {
    // … sort and return — Tier 2 is never reached
}
return r.findLatestLightweightTag(tagPrefix)  // only fires when len == 0
```

**Call site (no override possible):**

```go
// internal/cli/cli.go:288
latestTag, err := gitClient.FindLatestAnnotatedTag(cfg.TagPrefix)
// latestTag is used without modification at lines 375, 288–398
```

`FindLatestAnnotatedTag` is the sole method on the `GitClient` interface
(`internal/cli/cli.go:25`) that the CLI uses for baseline detection. No caller-side
override or post-filter exists.

---

## Decision Drivers

- **Correctness for migrated repos** — the primary failure mode is wrong baseline selection
  in repos moving from codfish. The fix must work for the migration case without
  per-repo manual intervention.
- **No git binary dependency** — `go-git` only; no shell-out.
- **Full history availability** — `fetch-depth: 0` is already enforced; all tags are
  locally reachable.
- **Idempotency** — re-running on the same HEAD must produce the same baseline. The sort
  key must be deterministic.
- **Tag prefix filtering** — `cfg.TagPrefix` must continue to scope the tag pool.
- **Resilience to garbage tags** — non-semver tags must be silently skipped, not crash.
- **Shallow-clone guard remains meaningful** — the `OpenRepo()` shallow-clone guard
  (returns `ShallowRepoError` → `main.go` exits with code 2 before any subcommand runs)
  must not be rendered vacuous by the chosen approach.
- **Interface isolation preserved** — `GitClient` in `internal/cli` is the change surface;
  `internal/cli` must not grow a direct go-github dependency.
- **Minimal operational surface** — avoid new required environment variables or secrets.
- **Correct in the steady-state (post-migration)** — once the repo is fully migrated and
  semrel has created its first annotated tag, the strategy must still produce the right
  baseline on every subsequent push.

---

## Options

### Option 1 — Semver-max across all tags (drop annotated/lightweight distinction)

**How it works:** Collect all tags (annotated and lightweight). Apply prefix filter. Parse
each name through `semver.ParseVersionFromTag`. Tags that fail to parse are silently
skipped. Take the tag with the highest semver value as the baseline. Tag type is irrelevant.
One pool, one sort key (semver magnitude), no type discrimination.

**Implementation surface:** The change is confined to `FindLatestAnnotatedTag` in
`internal/git/git.go`. The `GitClient` interface in `internal/cli/cli.go:24–33` does not
change (the method may be renamed to `FindLatestTag` or left as-is with updated
semantics — an implementation detail for coder). `internal/cli` is unaffected.

**Pros:**

- Eliminates the failure mode completely. The highest-semver tag wins regardless of whether
  a human, codfish, or semrel created it.
- Deterministic: semver ordering is nearly total, with a defined tiebreaker for the rare
  case where two tags parse to the same semver value (e.g. `v4.0` and `v4.0.0` both parse
  to `4.0.0` under Masterminds/semver). When two tags tie, prefer the tag with more
  dot-separated components in the name (`v4.0.0` beats `v4.0`; `v3.6.0` beats `v3.6`).
  Secondary sort key: `strings.Count(name, ".")` descending; final tiebreaker: `name`
  descending lexicographically. Timestamp-based sorts are susceptible to clock skew and
  timezone normalization edge cases — semver with this tiebreaker is not.
- Garbage-tag resilience is already solved: `semver.ParseVersionFromTag` returns an error
  for non-semver names, and the caller today already handles parse errors at
  `cli.go:375–378`. Skipping at collection time is the same logic moved earlier.
- Aligns with how every other semantic-release tool (semantic-release/semantic-release,
  Release Please, cargo-release) defines "last release": the highest published semver,
  regardless of git tag metadata.
- Tier 2 fallback becomes unnecessary: the single pool handles both tag types in one pass.
  The `findLatestLightweightTag` helper can be removed.
- The `OpenRepo()` shallow-clone guard (returns `ShallowRepoError` → `main.go` exits with
  code 2 before any subcommand runs) remains meaningful: a shallow clone still truncates
  commit history, causing `ListCommitsSinceTag` to walk past its stop commit and enumerate
  all commits. The guard fires before `FindLatestTag` is called.

**Cons:**

- Drops the annotated-tag preference. If someone pushes a garbage semver lightweight tag
  to a repository (e.g. `v99.0.0` as a test), it becomes the baseline permanently until
  a real release exceeds it.
  - **Mitigating factor:** This is true of any semver-based strategy, and the tag prefix
    filter (`cfg.TagPrefix`) already restricts the pool. The idempotency ladder at rungs 1
    and 2 (`internal/cli/cli.go:448–501`) catches the computed-version-already-exists case
    without crashing.
  - **Residual risk:** A malicious or accidental `v999.0.0` lightweight tag permanently
    blocks releases until manually deleted. This is a new risk not present in the current
    (broken) design — but the current design's "tag type" discrimination was never a
    meaningful proxy for "created by this tool," because codfish's alias tags, human-pushed
    tags, and semrel tags are all lightweight in migrated repos.
- `FindLatestAnnotatedTag` is named in `docs/architecture.md` as a load-bearing design
  decision. The name is wrong after this change; the architecture doc must be updated.
- The `IsAnnotated` field on `git.Tag` (used in `cli.go:297` for the lightweight-tag
  warning) will no longer gate the warning correctly if the chosen baseline happens to be
  lightweight post-migration. The warning's purpose was to signal "you might want to
  convert this tag." Post-fix, that warning fires on every run until semrel creates its
  first annotated tag — which is correct behavior but more verbose.

**Risks:**

- **Accidental high-semver tag:** An accidental `v9.0.0` or test tag entered as the
  baseline. Existing idempotency protects against double-release but not against version
  inflation. **Severity: low** — restricted by `cfg.TagPrefix`; operator must push the
  tag intentionally; the same risk exists in competing tools.
- **Aliasing confusion:** codfish alias tags (`v3.6`) parse to `3.6.0` under semver rules.
  A repo with `v3.6` (annotated, points to commit of `v3.6.2`) and `v3.6.2` (lightweight)
  would have `v3.6.2` as the semver winner, which is correct. **Severity: none.**

---

### Option 2 — GitHub Releases API as authoritative baseline

**How it works:** Replace tag-based detection with a call to the GitHub Releases API
(`GetLatestRelease` or `ListReleases` filtered to non-draft, non-prerelease). The tag name
on the returned Release object is parsed as the baseline version. Tag type is never
consulted.

**Implementation surface:** `GitHubClient` interface (`internal/cli/cli.go:35–42`) already
exposes `GetReleaseByTag`. A new method `GetLatestRelease` (or `ListReleases`) would be
needed. `cmdRelease` in `internal/cli/cli.go:221–579` would call this before the git tag
lookup (or replace it). The `GitClient.FindLatestAnnotatedTag` call at line 288 may be
removed or demoted to a fallback.

**Pros:**

- Authoritative for the "what did we officially release?" question. GitHub Releases are
  human-curated, not a side-effect of tag metadata conventions.
- Immune to accidental high-semver tags that are not backed by a GitHub Release.
- Works correctly for human-created releases (GitHub web UI creates both a tag and a
  Release object simultaneously).

**Cons:**

- **Requires a network call at baseline detection time.** `FindLatestAnnotatedTag` is
  currently a pure local-git operation. Adding a GitHub API call at this step introduces
  a new failure mode: GitHub API rate limits, transient network errors, and GitHub
  availability. A flaky API call would make the release pipeline flaky.
- **Breaks repos that have never created GitHub Releases** (only git tags). Bootstrap
  behavior and repos using `--dry-run` without pushing would see no releases and
  incorrectly fall through to the `nil` bootstrap path.
- **Does not fix the confirmed incident.** `v4.0.0` has a GitHub Release object
  (confirmed: published 2026-08-06T09:14:25Z, not draft, not prerelease — see prior
  findings). Yet semrel still computed `v3.7.0` from the `v3.6.0` baseline in the failing
  run. This means the API-based result (`v4.0.0`) and the current tag-based result (`v3.6`)
  would diverge — but the bug in the incident was specifically in the git-tag path. Option
  2 would fix the baseline selection but introduces a question: what does
  `ListCommitsSinceTag` do when the baseline's tag is from a GitHub Release whose git tag
  is lightweight? The commit-walk still depends on the tag's target SHA being reachable,
  which is unchanged.
- **Violates the go-git-only constraint.** The constraint is "no git binary dependency,"
  not "no network." However, adding a GitHub API dependency at baseline-detection time
  couples the git-introspection step to GitHub specifically — the tool cannot be tested
  or run against a plain git remote without a GitHub API endpoint.
- **`--dry-run` semantics are unchanged but the offline framing is false.** dry-run today
  already calls `GetReleaseByTag` unconditionally (`cli.go:449`). The substantive concerns
  with Option 2 are reliability and circular-dependency risk, not offline purity.
- **Circular dependency risk.** The idempotency check at rung 1 (`cli.go:448–501`) already
  calls `GetReleaseByTag`. If baseline detection also calls the Releases API, the two calls
  must be consistent — a race between a concurrent release and the baseline-detection call
  could produce incorrect results.
- **Governance gap.** The `GitHubClient` interface lives in `internal/cli`, not
  `internal/git`. Adding `GetLatestRelease` to `GitHubClient` to support baseline detection
  mixes two responsibilities: git tag resolution (a local concern) and GitHub Release state
  (a remote concern).

**Risks:**

- **API flakiness makes the release pipeline unreliable.** Every existing GitHub Action
  that uses the Releases API as a synchronization point has this failure mode. **Severity:
  high.**
- **Bootstrap regressions.** Existing test coverage for the bootstrap path assumes
  `FindLatestAnnotatedTag` returns `nil` when no tags exist. A GitHub API call returning
  `nil` for a different reason (no releases yet vs API error) is semantically ambiguous.
  **Severity: medium.**

---

### Option 3 — Semver-max within annotated tags, fall back to semver-max across all tags

**How it works:** Keep the annotated-preference structure, but remove the exclusivity.
After Tier 1 finds the best annotated tag (highest semver, not timestamp), check if any
lightweight tag has a higher semver. If so, the lightweight tag wins. Annotated tags are
still preferred within their own pool; a lightweight tag can only override upward, never
downward.

**Implementation surface:** Same as Option 1 — confined to `internal/git/git.go`. Two
sorting passes instead of one.

**Pros:**

- Preserves the original design intent: annotated tags are preferred. Repos that never
  had codfish and use only annotated tags behave identically to today (modulo sorting by
  semver instead of timestamp — see cons).
- Fixes the confirmed incident: `v4.0.0` (lightweight, semver `4.0.0`) > `v3.6` (annotated,
  semver `3.6.0`), so the lightweight tag wins.

**Cons:**

- **More complex than Option 1 with no additional correctness benefit.** The annotated
  preference is preserved, but the annotated-vs-lightweight distinction is not a
  meaningful proxy for "authoritative" in any migration scenario. A repo that has been
  fully migrated and running semrel for a year will have annotated `vX.Y.Z` tags (created
  by semrel) that are always the highest-semver tags. The "lightweight can override upward"
  escape hatch is only exercised in the migration window — and during that window it
  behaves identically to Option 1.
- **The sort key change from timestamp to semver for annotated tags is required regardless.**
  In the current code, Tier 1 sorts by `Author.When` (commit timestamp). This is incorrect
  even in the non-migration case: if two branches converge and the "older" branch's commit
  has a later `Author.When` (e.g. due to timezone or clock skew), the wrong annotated tag
  wins. Option 3 must fix the sort key to be semver-based anyway, making the annotated
  preference provide no additional guarantee of correctness.
- **Two-pass iteration over tags** (once for annotated, once for lightweight after
  comparison) adds marginal complexity and a second full iteration through all tags in
  the repo. In repos with thousands of tags this is observable but unlikely to be a
  performance concern.
- **Annotated alias tags (`v3.6`) parse to semver `3.6.0`**, which is correctly lower than
  `v4.0.0` (`4.0.0`). But an annotated alias tag `v4.0` (if codfish had created one)
  would parse as `4.0.0` — equal to the lightweight `v4.0.0`. The tiebreaker in a two-pool
  strategy must be defined (prefer annotated, prefer the later timestamp, prefer the patch
  tag over the alias). This edge case does not arise in Option 1 because the highest semver
  across one pool is unambiguous.
- **Increased cognitive load on maintainers.** The two-tier structure already confused the
  original author (the docstring at `git.go:96–97` says the fallback is "for repos
  migrating from codfish," which was the correct original intent but is now wrong in
  practice). Option 3 preserves a two-tier design that has already proven hard to reason
  about.

**Risks:**

- **Tiebreaker ambiguity** when an annotated alias tag (`v4.0`) and a lightweight patch
  tag (`v4.0.0`) both parse to `4.0.0`. **Severity: low** — the alias-tag convention is a
  codfish artifact; post-migration repos will not have new annotated aliases; the
  disambiguation rule (prefer annotated) is deterministic.
- **Future maintainer re-introduces the timestamp sort** without realizing the semver sort
  is now load-bearing. The two-tier structure obscures which invariants are in play.
  **Severity: low** — test coverage at the unit level would catch this regression.

---

## Recommendation

**Option 1 — Semver-max across all tags.**

In the context of a Go CLI that replaces `codfish/semantic-release` and operates on fully
cloned repositories (`fetch-depth: 0`), facing a confirmed bug where annotated alias tags
from the old workflow permanently hide all post-migration releases, we decided for
**semver-max across all tags** and against Option 2 (GitHub Releases API) and Option 3
(hybrid annotated-preference) to achieve a deterministic, offline-capable baseline
selection that works correctly in every migration scenario, accepting that:

- The `FindLatestAnnotatedTag` method name becomes semantically stale and must be
  updated (rename or updated semantics) and the architecture doc must be corrected.
- An accidental high-semver lightweight tag (e.g. a test or staging tag pushed without
  care) will inflate the baseline permanently, though the tag-prefix filter
  (`cfg.TagPrefix`) restricts the pool to the project's tag convention.
- The `IsAnnotated` warning in `cli.go:297–302` may fire more frequently during the
  migration window, which is acceptable (the warning is advisory).

**Why not Option 2:** Introduces a network dependency into what is currently a local-git
operation, creates new failure modes (API flakiness, rate limits), complicates dry-run
semantics, and does not eliminate the git-tag commit-walk dependency downstream
(`ListCommitsSinceTag` still requires the baseline tag's commit to be locally reachable).

**Why not Option 3:** Preserves a two-tier structure whose annotated/lightweight distinction
carries no meaningful signal in the migration context. Option 3 requires the same semver
sort as Option 1 for correctness and adds code complexity with no additional correctness
gain in the steady state. The "annotated preference" is only observable during the
migration window, and during that window Option 3 produces identical results to Option 1
(the highest-semver tag wins).

---

## Invariants

The following invariants must be preserved by any implementation of Option 1. These bind
regardless of implementation details:

1. **No git binary.** The implementation uses `go-git` exclusively. No shell-out to `git`.
   Verified: `internal/git/git.go` imports `go-git/go-git/v5` only; no `os/exec`.

2. **Full history required.** `fetch-depth: 0` and `fetch-tags: true` remain required.
   The shallow-clone guard fires at repository-open time (`OpenRepo()` → `ShallowRepoError`
   → `os.Exit(2)`) before `FindLatestTag` is ever called. A shallow clone still breaks
   `ListCommitsSinceTag` regardless of the baseline selection strategy.

3. **Idempotency.** Re-running on the same HEAD with the same tag set produces the same
   baseline. Semver ordering is nearly total, but Masterminds/semver treats `v4.0` and
   `v4.0.0` as equal (`Compare() == 0`). Because `sort.Slice` is not stable, a tie without
   a tiebreaker produces non-deterministic results. The implementation MUST apply the
   canonical tiebreaker: when two tags parse to equal semver values, prefer the tag with
   more dot-separated components in the name (`v4.0.0` beats `v4.0`; `v3.6.0` beats
   `v3.6`). Implementation: secondary sort key `strings.Count(name, ".")` descending, then
   `name` descending lexicographically as the final tiebreaker. This invariant is stronger
   under Option 1 than under the current timestamp sort (which is susceptible to clock skew).

4. **Tag prefix filter.** `cfg.TagPrefix` (verified: `internal/git/git.go:115–117`) must
   continue to scope the pool before any parse or sort operation. A tag that does not carry
   the prefix must not enter the candidate set.

5. **Garbage-tag resilience.** Tags whose names do not parse as valid semver after prefix
   stripping must be silently skipped (not cause a panic or early return). The existing
   `semver.ParseVersionFromTag` already returns an error for unparseable names; the
   implementation must discard those candidates, not propagate the error.

6. **Bootstrap path unchanged.** When the pool contains zero parseable semver tags (first
   release in a new repo, or a repo using a non-standard prefix), `FindLatestTag` returns
   `nil, nil`. `cmdRelease` already handles the `nil` case at `cli.go:352–372` (bootstrap
   via `cfg.InitialVersion`). This path must continue to work.

7. **`GitClient` interface signature.** `internal/cli` must not gain a direct `go-github`
   dependency as a result of this change. The `GitClient` interface is defined in
   `internal/cli/cli.go:24–33`. If the method is renamed, the mock in `cli_test.go:52`
   must be updated — this is an interface-isolation concern, not a new external dependency.

8. **`IsAnnotated` field on `git.Tag` remains populated.** `cli.go:297` reads
   `latestTag.IsAnnotated` to emit the advisory warning. This field must continue to be
   set correctly by `internal/git` so the warning can fire when the selected baseline is
   a lightweight tag.

---

## Consequences

**Positive:**

- The primary failure mode (migrated repos stuck on the pre-migration annotated alias
  baseline) is eliminated without per-repo manual remediation.
- Baseline detection is deterministic. Semver ordering is a total order; the current
  `Author.When` timestamp sort is not (two commits authored at the same second are
  unordered, and timezone normalization adds fragility).
- The two-tier structure and its associated maintenance surface (separate
  `findLatestLightweightTag` helper, the binary fork logic, the confusing docstring) can
  be replaced with a single pass.
- Aligns with how other semantic-release tools define "last release."
- No new environment variables, secrets, or external service dependencies.

**Negative:**

- An accidental or adversarial high-semver lightweight tag permanently inflates the
  baseline. This risk did not exist in the original design (which would ignore lightweight
  tags in any repo with annotated tags). Mitigation: `cfg.TagPrefix` restricts the pool;
  the operator must push such a tag deliberately; the idempotency ladder catches the
  computed-version-already-exists case without crashing or double-releasing.
- The `FindLatestAnnotatedTag` method name (and its documentation in `docs/architecture.md`)
  becomes stale. Renaming requires updating the `GitClient` interface, all implementations,
  the mock in `cli_test.go`, and the architecture doc. This is a mechanical refactor, not
  a behavioral risk — but it is a real cost.
- The advisory lightweight-tag warning (`cli.go:297–302`) fires on every run for
  migrated repos until semrel creates its first annotated tag. This is correct behavior
  but produces one additional log line per run during the migration window.

**Neutral:**

- The `findLatestLightweightTag` helper in `internal/git/git.go:157–205` becomes dead
  code. It must be removed to keep coverage clean and avoid confusion.
- Semver sorting replaces timestamp sorting. In the steady state (semrel-managed repo with
  only annotated patch tags) the two sort keys produce the same result, so there is no
  behavioral change for repos that never used codfish.
- `docs/architecture.md` must be updated to reflect that the baseline strategy is now
  semver-max across all tag types, and that the annotated/lightweight distinction is no
  longer part of the design. See Architecture Doc Delta below.

---

## Architecture Doc Delta

The following changes to `docs/architecture.md` are required to keep the document honest
after Option 1 is implemented. **This section specifies the delta — it does not edit the
document.**

### Section: "Package structure" (the `internal/git/` line)

Replace:

> `FindLatestAnnotatedTag`, `ListCommitsSinceTag`, `CreateAnnotatedTag`, `PushTag`

With:

> `FindLatestTag` (semver-max across all tag types), `ListCommitsSinceTag`,
> `CreateAnnotatedTag`, `PushTag`

### Section: "Design decisions" — new subsection to add

Add a subsection titled **"Version baseline detection: semver-max across all tags"**
after the existing "Idempotency ladder" section, with the following content:

> `FindLatestTag` collects all git tags matching `cfg.TagPrefix`, parses each as semver,
> discards non-parseable names, and returns the tag with the highest semver value. Tag type
> (annotated vs lightweight) is not a factor in selection.
>
> **Why:** The previous strategy (`FindLatestAnnotatedTag`) preferred annotated tags and
> fell back to lightweight only when zero annotated tags existed. This was correct for
> a clean install but broke for repos migrated from `codfish/semantic-release`: codfish
> created annotated `vMAJOR.MINOR` alias tags for every release, so any migrated repo has
> annotated aliases from its entire pre-migration history. These aliases permanently shadow
> post-migration releases (which produce only lightweight `vMAJOR.MINOR.PATCH` tags).
>
> **Sort key:** Semver ordering is a total order and is immune to clock skew, timezone
> normalization, and the `Author.When` vs `Committer.When` ambiguity that affected the
> previous timestamp-based sort.
>
> **ADR:** See `docs/architecture/adr-001-version-baseline-detection.md`.

### Section: "Shallow clone requirement"

Replace the shallow-clone sentence with:

> A shallow clone causes `OpenRepo()` to return `ShallowRepoError`. The startup code in
> `cmd/semrel/main.go` catches this error, logs a diagnostic, and exits with code 2 —
> `FindLatestTag` is never called.

### Section: "Idempotency ladder" — no change required

The three rungs of the idempotency ladder are unaffected by this change.

---

## Confirmation

This decision is confirmed correct when all of the following pass in CI:

1. **Existing test suite green:** `go test -race ./...` passes with no regressions. The
   mock in `cli_test.go` satisfies the updated `GitClient` interface.

2. **New unit test for Option 1 semantics:** A test in `internal/git/git_test.go` that
   creates a repo with (a) one old annotated alias tag (`v3.6`) and (b) one newer
   lightweight patch tag (`v4.0.0`) must assert that `FindLatestTag` returns `v4.0.0`,
   not `v3.6`.

3. **Migration-scenario integration test:** A test that mirrors
   `nrkno/iac-terraform-azure-postgres-flexible` — 14 annotated aliases (`v1.1`–`v3.6`),
   followed by lightweight `v3.6.0`, `v3.7.0`, `v3.8.0`, `v4.0.0` — asserts the baseline
   is `v4.0.0` and the computed next version is `v4.1.0` (given at least one `feat:` commit
   after `v4.0.0`'s target commit). The fixture must also include a lightweight `v4.0` tag
   pointing to a different commit than `v4.0.0`, confirming that the tiebreaker rule
   consistently selects `v4.0.0` over `v4.0`.

4. **Bootstrap path unchanged:** A test with zero semver-parseable tags asserts that
   `FindLatestTag` returns `nil, nil` and `cmdRelease` enters the bootstrap path.

5. **Tiebreaker test:** A repo with `v4.0` and `v4.0.0` both as lightweight tags pointing
   to different commits. Assert `FindLatestTag` consistently returns `v4.0.0` (patch-bearing
   name) regardless of tag iteration order. The test must exercise the tiebreaker path
   directly — both tags parse to semver `4.0.0` under Masterminds/semver.

6. **Architecture doc updated:** `docs/architecture.md` reflects the delta specified above
   before the PR is merged.
