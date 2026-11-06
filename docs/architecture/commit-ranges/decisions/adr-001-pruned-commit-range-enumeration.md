---
type: Architecture
title: "ADR-001: Pruned commit range enumeration"
description: Separate ancestry metadata discovery from bounded commit materialization while preserving release and PR lint policies.
tags: [git, commit-ranges, release, lint, adr]
timestamp: 2026-11-06
---

**Status:** accepted and implemented; focused object-read and production CLI composition proofs pass. Off-branch selected tags preserve the existing all-HEAD-history fallback.

## Decision

Use a parent-hash-only ancestry pass for the excluded ref, followed by go-git's existing committer-time iterator with that ancestry excluded **before parent objects are loaded**. Do not enumerate all HEAD history and then discard old commits. Read HEAD directly for release retry checks.

For example, if a PR branches at A, base advances to B, and the checkout is a synthetic merge of B and PR tip P, return the synthetic merge and PR-only commits. Exclude B and every ancestor of B. A feature commit with an old timestamp remains included if its ancestry puts it in the range.

This bounds application commit materialization, not all graph work. Building the exclusion set can still visit all base ancestors and use memory proportional to that ancestry. Object storage may decompress/cache whole objects even when callers read only their parent headers. There is no strict O(range) I/O or runtime promise.

## Derived from

- User assignment: release inspects commits since the last release; PR lint inspects only PR-range commits; avoid unnecessary full-history enumeration. Preserve existing event support, other lint modes, bootstrap, local operation, and tag selection.
- [Architecture](/docs/architecture.md): local go-git operations, no git binary, semver-max tag selection, full-clone requirement, and retry SHA comparison.
- [Baseline decision](/docs/architecture/adr-001-version-baseline-detection.md): historical rationale and invariants. Its status remains proposed; the current architecture and implementation independently describe the semver-max policy.
- [API reference](/docs/api-reference.md): lint contexts and release behavior. Some references to annotated-only selection are stale; they do not override the current semver-max architecture and assignment.

## Satisfies

| Obligation | Disposition |
| --- | --- |
| PR membership is reachable(HEAD) minus reachable(base), including synthetic merge HEAD | Satisfied by this decision |
| Release excludes previously released ancestry and includes merged commits regardless of timestamp | Satisfied when selected tag is ancestor of HEAD; otherwise existing all-HEAD fallback is preserved |
| Highest matching semver across annotated/lightweight tags, with existing tiebreakers | Satisfied elsewhere: `FindLatestTag`; unchanged |
| No selected tag means all reachable HEAD commits | Preserved; necessarily unbounded bootstrap |
| Existing other lint contexts, flags, and event support | Preserved in `cmdLint`; no dispatch redesign |
| No runtime git executable or network dependency for enumeration | Satisfied by existing go-git storage and iterators |
| Retry needs HEAD SHA, not history | Satisfied by direct reference lookup |
| Published-release enumeration and previous-tag policy | Outside scope; `ListCommitsBetweenTags` and `FindPreviousAnnotatedTag` unchanged |

## Decision question

How can the existing local Git wrapper compute graph-correct commit ranges without decoding and returning every HEAD ancestor just to filter most of them out?

## Drivers and constraints

- Timestamps determine presentation order, never membership or stopping conditions.
- Every parent edge matters. First-parent traversal and stopping the entire walk at one boundary are incorrect on merges.
- Keep selected baseline identity separate from range construction. Never silently choose a lower reachable tag.
- Conventional-commit parsing, release bump calculation, and PR enrichment receive only range commits.
- Return traversal/resolution errors; do not silently substitute bootstrap after failure.
- No new persistent graph cache, required commit-graph file, dependency upgrade, checkout optimization, API calls, event support, or logging contract.

## Options considered

1. **Keep full walks and filter output.** Least code, but still decodes every HEAD commit and cannot satisfy the enumeration correction. Stopping at the tag also drops older-timestamp merged commits. Rejected.
2. **Use full go-git Commit objects for the exclusion walk, then prune HEAD.** Reuses only public APIs and fixes membership, but still fully decodes messages/signatures for every historical base commit even though only parent hashes are needed. This is the simpler fallback if metadata-only reading proves unjustified, but it does not meet the intended separation as completely.
3. **Parent-only exclusion metadata plus existing pruned iterator.** A small local parent-header reader is added; range traversal/order stays with the existing dependency. Avoids full historical commit decoding at the application layer. Chosen.
4. **Bidirectional graph painting or a required on-disk commit graph.** Can reduce ancestry work, but adds graph algorithms or deployment/lifecycle requirements. No strict O(range) requirement justifies it. Rejected.

## Implementation-facing contracts

### Ancestry metadata

Within `internal/git`, build a hash set containing the excluded commit and all its ancestors. Use the repository's `EncodedObject(CommitObject, hash)` storage API and close every reader. Read the canonical tree header and contiguous parent headers only, then stop; do not scan message bodies, author/signature payloads, or trees/blobs. Validate header object IDs rather than silently coercing invalid hex. Align the parent block interpretation with go-git v5.19.2's `plumbing/object/commit_scanner.go` (`scanTree`, `scanParents`); later parent-like lines are not graph edges. Deduplicate before expanding each hash. Propagate storage/read/malformed graph-header errors.

This is a narrow graph-header extractor, not a second full commit decoder. Old message-body validity is not required to discover ancestry. Range commits still use go-git's normal full decoder. Reconsider this choice if the dependency exposes a suitable public parent-only decoder.

### Materialization and order

Resolve HEAD once for each range call. If its hash is excluded, return an empty range without decoding HEAD. Otherwise decode HEAD and use `object.NewCommitIterCTime` with the excluded hashes supplied through its exclusion input. It checks exclusions before loading each parent (`commit_walker_ctime.go:60–74`). Materialize `internal/git.Commit` only for yielded commits. Keep current field mapping and the dependency's current committer-time traversal order; do not introduce a global sort, author-time membership test, or tie-order promise.

`ListCommitsSinceRef` keeps existing ref resolution and returns exactly reachable(HEAD) minus reachable(resolved ref), including the synthetic merge commit when it is not on base. Unrelated roots return all HEAD commits as a consequence of the already accepted set-difference contract. All parent edges and duplicate suppression remain mandatory.

For a selected tag that is an ancestor of HEAD, `ListCommitsSinceTag` uses the same exclusion semantics. A nil tag retains the all-history bootstrap path. The selected tag target, not the annotated tag object's SHA, is the boundary.

### Retry HEAD lookup

Add `HeadSHA() (string, error)` to `Repository` and CLI `GitClient`. Return the resolved `raw.Head().Hash().String()` without a log iterator. Replace `internal/cli/cli.go:466–470` with this operation; retain the existing failure/conflict behavior and compare the tag target to the actual HEAD hash. Update test doubles. No commit ordering may be used as a proxy for HEAD identity.

### Fix locations and downstream documentation

- `internal/git/git.go:255–299`: replace single-stop release traversal, subject to off-branch clarification.
- `internal/git/git.go:327–393`: replace full base/full HEAD log filtering with metadata ancestry and pre-load exclusion.
- `internal/cli/cli.go:24–33,464–475`: direct HEAD interface and retry usage.
- `internal/git/git_test.go`, `internal/cli/cli_test.go`: concrete Git tests and CLI routing/consumer checks.
- Update `docs/architecture.md` SHA comparison and range explanation when implemented. Correct only affected API wording; do not broaden into unrelated documentation cleanup.

These are change surfaces and contracts, not an execution task sequence.

## Consequences

The range walker never follows an excluded parent. Historical metadata discovery remains potentially linear; this is explicit, not a hidden performance guarantee. The only new parsing responsibility is canonical parent-header extraction. No persisted state, rollout migration, or new operational prerequisite is introduced.

The correction also affects callers of the shared `ListCommitsSinceTag`, including `notes`; this is intrinsic to repairing that method. Do not apply the change to published-release enumeration incidentally.

## Confirmation and reconsideration

Verification evidence and remaining limits:

- PR split with base advancing after branch creation: exact SHA set, excluding old invalid base messages; include PR messages.
- PR merge-from-base and synthetic merge checkout: all base ancestry excluded; synthetic merge and all PR-only commits retained exactly once.
- Release merge where a feature commit's committer time predates the selected tag: feature retained, tag and all tag ancestors excluded. Include a side branch diverging before the release boundary to prove exclusion is not merely stopping at one hash.
- Selected annotated and lightweight tags, custom prefix, semver-max selection/tiebreakers, tag at HEAD (empty range), tagless bootstrap, unrelated PR roots, unresolved ref, missing graph object, and traversal error propagation.
- Header parser regression tests prove that a 32-KiB author field is not consumed in full, uppercase tree hashes accepted by go-git remain accepted, and a bare `parent` header is rejected. The fixture uses go-git's memory storer.
- Counting/instrumented storer: for both PR and tagged release ranges, excluded historical commit reads are limited to a small bounded prefix (at most 4096 bytes, including buffered read-ahead) of a 32-KiB-plus commit object, while the included range commit is fully read. This verifies bounded encoded-object reader consumption for the tested memory-storage fixtures, not zero body-byte read-ahead, zero filesystem decompression, or O(range) wall time.
- Retry conflict test uses a real repository's `HeadSHA` with timestamp-skewed history; CLI tag/commit selection inputs remain controlled by the existing mock client.
- PR lint and release dry-run composition tests use `git.OpenRepo` and the actual `cmdLint` / `cmdRelease`, with GitHub calls isolated by the existing interface. PR tests prove an invalid base is excluded and an invalid PR commit is rejected; the release test proves a historical breaking commit before the selected tag does not alter the bump.
- `go test -race ./...` and `go vet ./...` pass. Existing range tests cover graph membership including merge ancestry and off-branch fallback; this evidence is not a disk-I/O or performance benchmark.

Independent review should challenge exact membership and whether parent-only decoding really avoids the original expensive path. Focused instrumentation demonstrates bounded encoded-object reader consumption for the tested memory-storage fixtures; it does not measure disk decompression, cache behavior, or total runtime. Reconsider additional graph machinery only with measured unacceptable ancestry cost or a stronger accepted bound.

## Open questions and findings

**Off-branch release tag — existing fallback preserved by scope.** Semver-max selection considers all matching tags, not only HEAD ancestry. When the selected target is unreachable, `ListCommitsSinceTag` retains the prior all-HEAD-history fallback. This task does not improve or redefine off-branch eligibility; see the regression test for that compatibility behavior.

**Tool limitation — nonblocking.** Semantic navigation was attempted but code-intel returned a degraded Go backend due to incompatible export-data versions. Findings were grounded by direct source reads instead.

**Historical evidence note.** The original architecture review at baseline `a4af40e` preceded implementation and made no implementation or benchmark claim. Subsequent implementation and verification evidence is recorded above. `ListCommitsBetweenTags` has a similar stop-hash pattern but remains an unresolved adjacent issue outside this assignment, not a silently fixed guarantee.
