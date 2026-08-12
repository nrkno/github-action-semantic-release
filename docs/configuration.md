---
type: Configuration
title: semrel configuration reference
description: All environment variables consumed by semrel, subcommand flags, and GITHUB_OUTPUT fields written on a successful release.
tags: [configuration, environment-variables, flags, github-output]
timestamp: 2026-08-12
---

# Configuration

semrel is configured primarily through environment variables. All variables are
provided automatically by GitHub Actions; no manual setup is required for standard
workflows. In addition, the `lint` subcommand supports an optional
[`.semrelrc.yml` config file](#lint-configuration-file-semrelrcyml) for per-repo
rule overrides.

---

## Environment variables

### Authentication

| Variable | Required | Description |
| -------- | -------- | ----------- |
| `GITHUB_TOKEN` | Yes | GitHub token used for API calls and git push. Set automatically by GitHub Actions via `secrets.GITHUB_TOKEN`. |

### GitHub Actions context (set automatically)

| Variable | Description |
| -------- | ----------- |
| `GITHUB_REPOSITORY` | Owner and repo name in `owner/repo` format (e.g., `nrkno/myservice`). |
| `GITHUB_REF` | Full ref of the event (e.g., `refs/heads/main`, `refs/pull/42/merge`). Used to extract the PR number on `pull_request` events. |
| `GITHUB_REF_NAME` | Short ref name (e.g., `main`, `42/merge`). |
| `GITHUB_SHA` | SHA of the commit that triggered the workflow. |
| `GITHUB_EVENT_NAME` | Event type that triggered the workflow (`push`, `pull_request`, `release`, etc.). Used by `lint` to determine the commit range. On `push` events, lint reads pushed commits from the event payload file. |
| `GITHUB_BASE_REF` | Target branch of a pull request. Only set on `pull_request` events. Used by `lint` to define the range start. |
| `GITHUB_OUTPUT` | Path to the step output file. semrel appends `key=value` lines here when outputs are available. |
| `GITHUB_API_URL` | GitHub API base URL. Defaults to `https://api.github.com`. Override for GitHub Enterprise Server. |
| `GITHUB_SERVER_URL` | GitHub server base URL. Defaults to `https://github.com`. Override for GitHub Enterprise Server. |
| `GITHUB_ACTIONS` | Set to `true` inside a GitHub Actions runner. semrel uses this to detect whether it is running in CI. |
| `GITHUB_RUN_ID` | Unique ID of the current workflow run (informational). |
| `GITHUB_ACTOR` | GitHub username of the person or app that triggered the run (informational). |

### GitHub Action inputs (`INPUT_*`)

When semrel runs as a GitHub Action the runner automatically maps every
`inputs:` entry declared in `action.yml` to an `INPUT_<NAME>` environment
variable (uppercased, hyphens replaced by underscores). `entrypoint.sh`
reads these and translates them to CLI arguments.

| Variable | Maps to | Description |
| -------- | ------- | ----------- |
| `INPUT_SUBCOMMAND` | first positional argument | Subcommand to execute: `lint`, `release`, `notify`, or `notes`. Required. |
| `INPUT_TOKEN` | `GITHUB_TOKEN` env var | GitHub token. Overrides the value injected by the runner. Defaults to `github.token`. |
| `INPUT_DRY_RUN` | `--dry-run` flag | Set to `"true"` to skip tag creation, push, and GitHub Release creation. |
| `INPUT_WORKING_DIRECTORY` | `cd` target | Path to the repository root before running the binary. Defaults to `.` (workspace root). |

These variables are only relevant when semrel is used via `action.yml`. Direct
CLI invocations do not use `INPUT_*`; they accept flags and environment variables
directly as documented in the sections below.

---

### semrel-specific

| Variable | Scope | Description |
| -------- | ----- | ----------- |
| `SEMREL_LOG_LEVEL` | all subcommands | Log verbosity: `DEBUG`, `INFO`, `WARN`, `ERROR` (case-insensitive). Defaults to `INFO`. Set `DEBUG` to log binary version, config file fields, git operation timings, and PR lookup details. |
| `SEMREL_TAG` | `notify` | Full tag name of the published release (e.g. `v1.3.0`). Required for `semrel notify`. Set from `${{ github.event.release.tag_name }}`. |
| `SEMREL_RELEASE_URL` | `notify` | HTML URL of the published GitHub Release. Constructed from `GITHUB_SERVER_URL` + `GITHUB_REPOSITORY` + `SEMREL_TAG` if absent. |
| `SEMREL_VERSION` | `notify` | Version string without `v` prefix (e.g., `1.2.3`). Optional; used for display only. |

These `SEMREL_TAG`, `SEMREL_RELEASE_URL`, and `SEMREL_VERSION` variables are set
explicitly in the `notify.yml` workflow from the GitHub release event payload and
passed to `semrel notify`.

---

## Subcommand flags

### `lint`

| Flag | Type | Default | Description |
| ---- | ---- | ------- | ----------- |
| `--from-ref` | string | auto-detected | Start of the commit range. Defaults to `GITHUB_BASE_REF` on `pull_request` events. On `push` events, automatic mode reads pushed commits from `GITHUB_EVENT_PATH`; explicit `--from-ref` overrides that and uses a git range. On `release` events, defaults to the latest annotated tag. |
| `--to-ref` | string | `HEAD` | End of the commit range. |

### `release`

| Flag | Type | Default | Description |
| ---- | ---- | ------- | ----------- |
| `--dry-run` | bool | `false` | Preview mode. Skips the branch guard (so it can run on PR branches), computes the projected next version from conventional commits, and writes all output fields — but performs zero write operations (no tag creation, no push, no GitHub Release). Use on pull requests to preview what version would be released when the branch merges. |

### `notify`

No flags. All configuration comes from environment variables.

### `notes`

No flags. All configuration comes from environment variables.

---

## GITHUB_OUTPUT fields

The `release` subcommand writes the following fields to `GITHUB_OUTPUT` on both
success (release created) and no-op (release already existed):

| Field | Example value | Description |
| ----- | ------------- | ----------- |
| `released` | `true` / `false` | Whether a new release was created in this run. |
| `version` | `1.2.3` | Version string without the `v` prefix. |
| `tag` | `v1.2.3` | Git tag name (always `v`-prefixed). |
| `major` | `1` | Major version component. |
| `minor` | `2` | Minor version component. |
| `patch` | `3` | Patch version component. |

When `released=false` (no-op run), all version fields still reflect the **current**
version (not empty), so downstream jobs can always read `steps.semrel.outputs.version`
without a null-check.

The `notes` subcommand writes one additional field:

| Field | Description |
| ----- | ----------- |
| `notes` | Full Markdown release notes body. |

---

## Lint configuration file (`.semrelrc.yml`)

`semrel lint` looks for an optional `.semrelrc.yml` file in the working directory
(the repo root, or `INPUT_WORKING_DIRECTORY` if that variable is set). When found,
fields in the file override the built-in rule defaults; any field that is absent
retains its default value.

### File location

Place `.semrelrc.yml` at the **root of the repository** being linted. If you set
`INPUT_WORKING_DIRECTORY` in the GitHub Action, place it at the root of that
directory instead.

### Full YAML schema

```yaml
# Version bump level overrides. Valid levels: major, minor, patch, none.
# Absent keys default to none; only listed keys are overridden.
bump-rules:
  breaking-change: major
  feat: minor
  fix: patch

# Branch name patterns that allow `semrel release` to proceed (path.Match syntax).
release-branches: [main, master]

# String prepended to the version number when creating git tags.
tag-prefix: "v"

commit-types:
  # Additional commit types accepted by `semrel lint`, added to the built-in set.
  extra-types: []
  # Full replacement for the built-in type set. Takes precedence over extra-types.
  allowed-types: []

# Baseline version for the bootstrap case (first release, no annotated tags).
initial-version: "0.0.0"

lint:
  rules:
    # Fail if a commit description starts with an uppercase letter.
    # Default: true
    capital-first-letter: true

    # Fail if a commit has no scope (e.g. "feat: …" without "(scope)").
    # Default: false
    require-scope: false
```

### Rule reference

#### `capital-first-letter` (default: `true`)

Requires that the description part of a conventional commit begins with a
lowercase letter.

| Commit | Result |
| ------ | ------ |
| `feat: add login page` | ✅ Pass — description starts with lowercase |
| `feat: Add login page` | ❌ Fail — description starts with uppercase |

Set to `false` to allow uppercase descriptions:

```yaml
lint:
  rules:
    capital-first-letter: false
```

#### `require-scope` (default: `false`)

When enabled, every commit must include a scope in parentheses after the type.

| Commit | Result |
| ------ | ------ |
| `feat(auth): add login page` | ✅ Pass — scope `auth` is present |
| `feat: add login page` | ❌ Fail — no scope provided |

Enable with:

```yaml
lint:
  rules:
    require-scope: true
```

### Top-level field reference

#### `bump-rules` (map, default: `{breaking-change: major, feat: minor, fix: patch}`)

Maps commit types — and the sentinel `breaking-change` — to version bump levels.
Valid levels: `major`, `minor`, `patch`, `none`.

Absent keys default to `none`. Entries overlay the defaults; specifying only
`chore: patch` keeps `feat→minor`, `fix→patch`, and `breaking-change→major` unchanged.

To suppress all bumps ("freeze" mode), set every key to `none` explicitly.
A bare `bump-rules:` key (YAML null) restores the three defaults.

Example — add patch bump for chore and suppress fix bumps:
```yaml
bump-rules:
  chore: patch
  fix: none
```

#### `release-branches` (list, default: `[main, master]`)

Branch name patterns that allow `semrel release` to proceed. Uses `path.Match`
syntax. `*` matches a single path segment and does NOT cross `/` boundaries.

Example — single branch:
```yaml
release-branches: [main]
```

#### `tag-prefix` (string, default: `"v"`)

String prepended to the version number when creating git tags.
Default `"v"` produces tags like `v1.2.3`.
Set `""` for bare version tags (`1.2.3`).

**Important**: changing this on a repository with existing `v`-prefixed tags
will cause semrel to stop finding those tags. Set this only for new repositories
or when migrating the entire tag history.

#### `commit-types.extra-types` (list, default: `[]`)

Additional commit types accepted by `semrel lint`, added on top of the built-in
set (feat, fix, chore, docs, ci, refactor, test, perf, build, revert).

Example:
```yaml
commit-types:
  extra-types: [deps, security]
```

#### `commit-types.allowed-types` (list, default: `[]`)

Full replacement for the built-in type set. When non-empty, `semrel lint`
only accepts the listed types. Takes precedence over `extra-types`.

#### `initial-version` (string, default: `"0.0.0"`)

Baseline version for the bootstrap case (first release when no annotated tags exist).
The detected bump is applied on top of this value.

| initial-version | first fix → | first feat → | first breaking → |
|---|---|---|---|
| `0.0.0` (default) | `v0.0.1` | `v0.1.0` | `v1.0.0` |
| `1.0.0` | `v1.0.1` | `v1.1.0` | `v2.0.0` |

### Behaviour when the file is absent

If `.semrelrc.yml` is not present, all default rules apply unchanged. No error
is produced.

### Behaviour when the file is malformed

If the file exists but contains invalid YAML or does not conform to the expected
schema, `semrel lint` exits with code `1` and prints a clear error message to
stderr. Fix the file and re-run.

### Working-directory note

If `INPUT_WORKING_DIRECTORY` is set (e.g. `working-directory: services/my-svc`
in your Action step), semrel changes to that directory before running `lint`.
The `.semrelrc.yml` file is therefore resolved relative to that path, **not**
the workflow workspace root.

---

## Version bump rules

The next version is calculated from the highest-impact commit type in the range:

| Commit signal | Bump |
| ------------- | ---- |
| Any commit with `BREAKING CHANGE:` footer or `!` suffix | major |
| `feat` | minor |
| `fix`, `perf`, `refactor` | patch |
| `docs`, `style`, `test`, `build`, `ci`, `chore` | none (no release) |

Bootstrap (no previous annotated tag): first release is always `v0.0.1`.
