# Release image pinning

- [ADR-001: Off-branch digest tag with ancestry-difference ranges](/docs/architecture/release-image-pinning/decisions/adr-001-off-branch-digest-tag-with-ancestry-ranges.md) — The release tag points at a child of `main` whose `action.yml` holds the signed image digest; semrel counts commits reachable from HEAD and not from the tag, so off-branch tags no longer cause full-history re-releases.
