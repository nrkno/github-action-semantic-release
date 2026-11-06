---
type: Log
title: Architecture decisions change log
description: Chronological record of changes to the docs/architecture/ bundle.
tags: [changelog, log, okf, architecture]
timestamp: 2026-11-06
---

# Architecture decisions update log

## 2026-11-06

**Creation**: `docs/architecture/commit-ranges/index.md` and `docs/architecture/commit-ranges/decisions/adr-001-pruned-commit-range-enumeration.md` — Proposed parent-only ancestry discovery and pre-load range exclusion; records unresolved off-branch release semantics.
**Update**: `docs/architecture/index.md` — Added commit-range topic and architecture log navigation.
**Update**: `docs/architecture.md` and commit-range ADR — Implemented parent-only ancestry discovery and pre-load exclusion, direct HEAD SHA retry lookup, and documented preserved off-branch fallback.
**Update**: commit-range ADR — Recorded passing bounded object-read tests for PR/release ranges and real Git/CLI composition tests for PR lint and release dry-run; disk I/O and runtime bounds remain unclaimed.
**Update**: commit-range ADR — Added reviewer regressions for oversized author headers, uppercase tree IDs, and missing parent IDs; documented invalid PR commit rejection, historical breaking-commit exclusion, and timestamp-skew retry conflict evidence. Corrected the historical pre-implementation evidence note.

## 2026-08-19

**Creation**: `docs/architecture/adr-001-version-baseline-detection.md` — ADR evaluating three options for version baseline detection (semver-max all tags, GitHub Releases API, hybrid annotated-preference) and recommending semver-max across all tags to fix the confirmed migration-scenario bug.
**Creation**: `docs/architecture/index.md` — bundle navigation index.
**Creation**: `docs/architecture/log.md` — this log file.
