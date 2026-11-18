# semrel architecture decisions

* [ADR-001: Version baseline detection strategy](adr-001-version-baseline-detection.md) - Decides how semrel selects the "last release" baseline version, resolving a bug where the annotated/lightweight distinction permanently hides post-migration releases in repos migrated from codfish/semantic-release.
* [Commit range correction](/docs/architecture/commit-ranges/index.md) — Implemented graph-correct commit-range enumeration; existing off-branch fallback is preserved.
* [Release image pinning](/docs/architecture/release-image-pinning/index.md) — Digest-pinned `action.yml` on an off-branch tagged commit; replaces the off-branch all-history fallback with ancestry-difference ranges (accepted and implemented).
* [Architecture change log](/docs/architecture/log.md)
