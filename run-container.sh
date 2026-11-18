#!/usr/bin/env bash
set -euo pipefail

# Forward only what semrel consumes. Docker env files are line-oriented;
# multiline values cannot be forwarded without changing their meaning.
vars=(
  GITHUB_TOKEN GITHUB_API_URL GITHUB_REPOSITORY GITHUB_REF GITHUB_REF_NAME
  GITHUB_SHA GITHUB_EVENT_NAME GITHUB_BASE_REF GITHUB_EVENT_PATH GITHUB_OUTPUT
  GITHUB_RUN_ID GITHUB_ACTOR GITHUB_SERVER_URL GITHUB_ACTIONS SEMREL_LOG_LEVEL
  INPUT_SUBCOMMAND INPUT_DRY_RUN INPUT_WORKING_DIRECTORY SEMREL_TAG SEMREL_RELEASE_URL
)
umask 077
envfile="$(mktemp "$RUNNER_TEMP/semrel-env.XXXXXX")"
trap 'rm -f "$envfile"' EXIT
for name in "${vars[@]}"; do
  value="${!name-}"
  if [[ "$value" == *$'\n'* || "$value" == *$'\r'* ]]; then
    echo "::error::Value of $name must not contain a newline"
    exit 1
  fi
  printf '%s=%s\n' "$name" "$value" >> "$envfile"
done

# Use the runner's uid/gid so it can write the checkout and step output.
# Identity-mount just the workspace, output and read-only event file, not
# RUNNER_TEMP as a whole, so semrel does not get other runner state.
docker run --rm \
  --user "$(id -u):$(id -g)" \
  --read-only \
  --tmpfs /tmp \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --env-file "$envfile" \
  -v "$GITHUB_WORKSPACE:$GITHUB_WORKSPACE" \
  -v "$GITHUB_OUTPUT:$GITHUB_OUTPUT" \
  -v "$GITHUB_EVENT_PATH:$GITHUB_EVENT_PATH:ro" \
  -w "$GITHUB_WORKSPACE" \
  "$ACTION_IMAGE"
