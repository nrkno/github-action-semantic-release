## 1. Changelog

**Guiding Principles:**

- Changelogs are for humans, not machines.
- There should be an entry for every single version.
- The same types of changes should be grouped.
- Versions and sections should be linkable.
- The latest version comes first.
- The release date of each version is displayed.
- Mention whether you follow Semantic Versioning.
- Types of changes
- Added for new features.
- Changed for changes in existing functionality.
- Deprecated for soon-to-be removed features.
- Removed for now removed features.
- Fixed for any bug fixes.
- Security in case of vulnerabilities.

---

## 2. Using gopls in This Repo

**gopls is registered as an MCP server in this opencode environment. Use it for Go symbol work — it is faster and more accurate than grep for this codebase.**

Reach for gopls instead of Read/Grep when:

- **Finding where a function or type is defined** → `gopls_go_search` to locate it, then `gopls_go_file_context` for cross-file dependencies
- **Finding all callers of a function** → `gopls_go_symbol_references` (e.g. all callers of `semver.ParseVersion`)
- **Checking what a package exports** → `gopls_go_package_api` with the import path
- **Checking for build errors or type diagnostics** → `gopls_go_diagnostics`
- **Renaming a symbol safely across the repo** → `gopls_go_rename_symbol`

Do **not** reach for gopls to read file content, check test output, or run builds — use `Read` and `Bash` for those.

---

## 3. Build, Test & Verify

```bash
# Build
go build ./cmd/semrel
CGO_ENABLED=0 go build -o bin/semrel ./cmd/semrel   # static binary (production)

# Test — always with race detector
go test -race ./...
go test -race -shuffle=on -count=3 ./cmd/semrel/...   # integration: shuffle for order-independence

# Vet + lint
go vet ./...
golangci-lint run

# Vulnerability check
govulncheck ./...
```

**Pre-commit:** `go test -race ./...` and `go vet ./...` must both be clean before pushing.

---

## 4. OKF Documentation Standard

The full standard is in `docs/okf-standard.md` (queryable via `get_doc(topic="okf standard")`). Key rules for agents modifying docs in this repo:

- Every `docs/*.md` file (except `index.md`) must have YAML frontmatter with a non-empty `type:` field
- `index.md` has **no frontmatter** — required by OKF spec
- Valid `type` values: `Architecture`, `Playbook`, `Configuration`, `API Reference`, `Metrics Reference`, `Log`
- Tags: lowercase, hyphenated strings
- Cross-links: bundle-relative paths (e.g. `/docs/file.md`)

### Update obligation

Any code change that alters documented behavior triggers this section — you do not have to be told separately. If a change affects tool behavior, configuration, or deployment, update the relevant `docs/*.md` in the SAME commit as the code change. When modifying any `docs/` file: (1) update its `timestamp:` field to the current date; (2) add a dated entry to `docs/log.md` (newest first, bold action prefix: `**Update**`, `**Creation**`, `**Migration**`, `**Deprecation**`).


