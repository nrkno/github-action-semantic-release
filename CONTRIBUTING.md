# Contributing to semrel

Thank you for your interest in contributing. This document covers the workflow,
commit requirements, and local development setup.

---

## Code of conduct

Be kind and constructive. See the [NRK open-source guidelines](https://opensource.nrk.no/).

---

## Conventional commits — required

Every commit merged to `main` **must** follow the
[Conventional Commits 1.0](https://www.conventionalcommits.org/en/v1.0.0/) specification.
The CI `lint-commits` job runs `./semrel lint` against your PR and will fail if any commit
does not comply.

Accepted types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`.

A breaking change uses `!` after the type or a `BREAKING CHANGE:` footer:

```
feat!: drop support for Go 1.21
```

```
feat: new flag --reuse-tag

BREAKING CHANGE: --force-tag removed; use --reuse-tag instead
```

---

## Pull request process

1. **Fork** the repository and create a branch from `main`.
2. **Write tests** for any changed behaviour. The test suite includes unit tests
   (in-memory go-git repos, httptest mocks) and integration tests
   (`-tags=integration`).
3. **Run checks locally** (see below) before opening a PR.
4. **Open the PR** against `main`. The CI pipeline (tests, race detector, lint,
   commit lint) must pass before merge.
5. **Squash-merge** is preferred so that the merge commit itself is a valid
   conventional commit and triggers the correct version bump.

---

## Local development setup

### Prerequisites

- Go 1.26+
- Docker (optional, for container builds)
- [golangci-lint](https://golangci-lint.run/usage/install/) (optional, for local lint)

### Build

```bash
go build -o semrel ./cmd/semrel
```

### Run tests

```bash
# Unit tests with race detector
go test ./... -count=1 -race

# Integration tests (requires network access to GitHub API — set GITHUB_TOKEN)
go test ./... -count=1 -tags=integration
```

### Run linters

```bash
go vet ./...
golangci-lint run
```

### Try semrel locally

```bash
# In a git repo with conventional commits:
./semrel lint
./semrel release --dry-run
./semrel notes
```

---

## Project structure

```
cmd/semrel/          Entry point — wires clients and executes cobra root
internal/
  cli/               Cobra subcommand handlers (lint, release, notify, notes)
  conventional/      Conventional commit parser and validator
  env/               GitHub Actions environment variable loader
  git/               go-git wrapper (tags, commits, push)
  github/            GitHub REST API client wrapper
  notes/             Release notes generator
  output/            GITHUB_OUTPUT writer
  semver/            Semver parsing, bump detection, version arithmetic
```

---

## Releasing

Releases are handled automatically by the release workflow when a conventional commit
is merged to `main`. You do not need to manually create tags or GitHub Releases.
