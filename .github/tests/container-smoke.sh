#!/usr/bin/env bash
set -euo pipefail

runner="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/run-container.sh"
tmp="$(mktemp -d "${RUNNER_TEMP:-/tmp}/semrel-smoke.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/workspace/test-repo" "$tmp/runner"
repo="$tmp/workspace/test-repo"
git init --quiet "$repo"
git -C "$repo" -c user.name=CI -c user.email=ci@example.invalid \
  -c commit.gpgsign=false commit --quiet --allow-empty -m 'Initial history'
before="$(git -C "$repo" rev-parse HEAD)"
git -C "$repo" -c user.name=CI -c user.email=ci@example.invalid \
  -c commit.gpgsign=false commit --quiet --allow-empty -m 'fix: exercise the container'
after="$(git -C "$repo" rev-parse HEAD)"

event="$tmp/runner/event.json"
output="$tmp/runner/output"
printf '{"before":"%s","after":"%s","commits":[{"id":"%s","message":"fix: exercise the container"}]}\n' \
  "$before" "$after" "$after" > "$event"
: > "$output"

run_semrel() {
  # No credentials or Git remote; even dry-run API lookups stay on loopback.
  env -i PATH="$PATH" HOME="$HOME" \
    RUNNER_TEMP="$tmp/runner" \
    GITHUB_WORKSPACE="$tmp/workspace" \
    GITHUB_EVENT_PATH="$event" \
    GITHUB_OUTPUT="$output" \
    GITHUB_SERVER_URL=https://github.com \
    GITHUB_API_URL=http://127.0.0.1:1 \
    GITHUB_REPOSITORY=example/semrel-smoke \
    GITHUB_EVENT_NAME=push GITHUB_REF_NAME=main \
    INPUT_WORKING_DIRECTORY=test-repo \
    INPUT_SUBCOMMAND="$1" INPUT_DRY_RUN="$2" ACTION_IMAGE="$image" \
    bash "$runner"
}

image="${1:?usage: container-smoke.sh LOCAL_IMAGE}"
# Without the event payload, lint falls back to history and rejects its first commit.
run_semrel lint false
run_semrel release true
grep -Fx 'released=false' "$output"
grep -Fx 'tag=v0.0.1' "$output"
grep -Fx 'bump=patch' "$output"
if [[ -n "$(git -C "$repo" tag --list)" ]]; then
  echo "Dry run unexpectedly created a tag" >&2
  exit 1
fi
if compgen -G "$tmp/runner/semrel-env.*" > /dev/null; then
  echo "Container runner did not remove its env file" >&2
  exit 1
fi
