#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENTRYPOINT="$ROOT_DIR/docker/daemon-pi-entrypoint.sh"
FIXTURES="$ROOT_DIR/scripts/testdata/daemon-pi"
TEST_ROOT="$(mktemp -d)"
trap 'chmod -R u+w "$TEST_ROOT" 2>/dev/null || true; rm -rf "$TEST_ROOT"' EXIT

mkdir -p "$TEST_ROOT/bin" "$TEST_ROOT/config" "$TEST_ROOT/home" "$TEST_ROOT/workspaces"
cp "$FIXTURES/fake-pi" "$TEST_ROOT/bin/pi"
cp "$FIXTURES/fake-dars" "$TEST_ROOT/bin/dars"
chmod +x "$TEST_ROOT/bin/pi" "$TEST_ROOT/bin/dars"
printf '{"defaultProvider":"xai","defaultModel":"grok-4.6"}\n' >"$TEST_ROOT/config/settings.json"

NODE_DIR="$(dirname "$(command -v node)")"
BASE_PATH="$TEST_ROOT/bin:$NODE_DIR:/usr/bin:/bin"

run_entrypoint() {
  local output="$1"
  shift
  local -a base_env=(
    "PATH=$BASE_PATH"
    "HOME=$TEST_ROOT/home"
    "DARS_SERVER_URL=http://dars.test"
    "DARS_WORKSPACE_ID=10000000-0000-0000-0000-000000000001"
    "DARS_DAEMON_ID=daemon-pi-test"
    "DARS_DAEMON_TOKEN=ddt_environment_secret"
    "DARS_PI_PATH=$TEST_ROOT/bin/pi"
    "DARS_PI_MODEL=xai/grok-4.6"
    "PI_CODING_AGENT_DIR=$TEST_ROOT/config"
    "DARS_WORKSPACES_ROOT=$TEST_ROOT/workspaces"
    "FAKE_DAEMON_PI_LOG=$TEST_ROOT/calls.log"
  )
  if ! env -i "${base_env[@]}" "$@" bash "$ENTRYPOINT" >"$output.stdout" 2>"$output.stderr"; then
    sed 's/ddt_[[:alnum:]_.-]*/<redacted>/' "$output.stderr" >&2
    return 1
  fi
}

: >"$TEST_ROOT/calls.log"
run_entrypoint "$TEST_ROOT/basic" DARS_PI_PREFLIGHT=basic
grep -Fxq 'dars daemon auth-preflight' "$TEST_ROOT/calls.log"
grep -Fxq 'dars daemon start --foreground' "$TEST_ROOT/calls.log"
if grep -Fq 'ddt_environment_secret' "$TEST_ROOT/basic.stderr"; then
  echo "entrypoint leaked Daemon Token"
  exit 1
fi

: >"$TEST_ROOT/calls.log"
run_entrypoint "$TEST_ROOT/active" DARS_PI_PREFLIGHT=active
grep -Fq -- '--no-approve --model xai/grok-4.6 --no-session --no-tools --no-context-files --mode json -p' "$TEST_ROOT/calls.log"

: >"$TEST_ROOT/calls.log"
if run_entrypoint "$TEST_ROOT/invalid-active" DARS_PI_PREFLIGHT=active FAKE_PI_AUTH_STATUS=invalid; then
  echo "invalid built-in Pi auth unexpectedly passed active preflight"
  exit 1
fi
if grep -Fq -- '--no-approve' "$TEST_ROOT/calls.log"; then
  echo "invalid built-in Pi auth unexpectedly reached active probe"
  exit 1
fi

: >"$TEST_ROOT/calls.log"
if run_entrypoint "$TEST_ROOT/custom-basic" DARS_PI_PREFLIGHT=basic DARS_PI_MODEL=custom/extension-model FAKE_PI_PROVIDER=custom FAKE_PI_MODEL=extension-model FAKE_PI_AUTH_STATUS=invalid FAKE_PI_CUSTOM_PROVIDER=1; then
  echo "custom Provider unexpectedly passed basic preflight"
  exit 1
fi
grep -Fq 'requires DARS_PI_PREFLIGHT=active' "$TEST_ROOT/custom-basic.stderr"

: >"$TEST_ROOT/calls.log"
run_entrypoint "$TEST_ROOT/custom-active" DARS_PI_PREFLIGHT=active DARS_PI_MODEL=custom/extension-model FAKE_PI_PROVIDER=custom FAKE_PI_MODEL=extension-model FAKE_PI_AUTH_STATUS=invalid FAKE_PI_CUSTOM_PROVIDER=1
grep -Fq -- '--no-approve --model custom/extension-model --no-session --no-tools --no-context-files --mode json -p' "$TEST_ROOT/calls.log"

: >"$TEST_ROOT/calls.log"
run_entrypoint "$TEST_ROOT/disabled" DARS_PI_PREFLIGHT=disabled
if grep -Eq 'pi (--list-models|auth )' "$TEST_ROOT/calls.log"; then
  echo "disabled Pi preflight unexpectedly checked catalog or auth"
  exit 1
fi

: >"$TEST_ROOT/calls.log"
empty_token="$TEST_ROOT/empty-token"
: >"$empty_token"
if run_entrypoint "$TEST_ROOT/file-fail" DARS_DAEMON_TOKEN_FILE="$empty_token"; then
  echo "authoritative empty token file unexpectedly fell back"
  exit 1
fi
if grep -Fq 'dars ' "$TEST_ROOT/calls.log"; then
  echo "DARS started after authoritative token-file failure"
  exit 1
fi

: >"$TEST_ROOT/calls.log"
if run_entrypoint "$TEST_ROOT/model-fail" FAKE_PI_CATALOG_MISSING=1; then
  echo "missing Pi model unexpectedly passed"
  exit 1
fi
if grep -Fq 'dars ' "$TEST_ROOT/calls.log"; then
  echo "DARS started after Pi model failure"
  exit 1
fi

: >"$TEST_ROOT/calls.log"
if run_entrypoint "$TEST_ROOT/auth-fail" FAKE_PI_AUTH_STATUS=invalid; then
  echo "invalid Pi auth unexpectedly passed basic preflight"
  exit 1
fi

: >"$TEST_ROOT/calls.log"
if run_entrypoint "$TEST_ROOT/active-fail" DARS_PI_PREFLIGHT=active FAKE_PI_ACTIVE_FAIL=1; then
  echo "failed Pi active probe unexpectedly passed"
  exit 1
fi

: >"$TEST_ROOT/calls.log"
if run_entrypoint "$TEST_ROOT/sentinel-fail" DARS_PI_PREFLIGHT=active FAKE_PI_ACTIVE_SENTINEL=0; then
  echo "Pi active probe without sentinel unexpectedly passed"
  exit 1
fi

: >"$TEST_ROOT/calls.log"
if run_entrypoint "$TEST_ROOT/dars-fail" FAKE_DARS_AUTH_FAIL=1; then
  echo "failed DARS auth unexpectedly started daemon"
  exit 1
fi
if grep -Fq 'dars daemon start --foreground' "$TEST_ROOT/calls.log"; then
  echo "daemon started after DARS auth failure"
  exit 1
fi

chmod -R a-w "$TEST_ROOT/config"
: >"$TEST_ROOT/calls.log"
run_entrypoint "$TEST_ROOT/readonly" DARS_PI_PREFLIGHT=basic
grep -Fq -- '--no-refresh' "$TEST_ROOT/calls.log"
chmod -R u+w "$TEST_ROOT/config"

chmod a-x "$TEST_ROOT/bin/pi"
: >"$TEST_ROOT/calls.log"
if run_entrypoint "$TEST_ROOT/binary-fail"; then
  echo "non-executable Pi binary unexpectedly passed"
  exit 1
fi
if grep -Fq 'dars ' "$TEST_ROOT/calls.log"; then
  echo "DARS started after Pi binary failure"
  exit 1
fi
chmod +x "$TEST_ROOT/bin/pi"

bash -n "$ENTRYPOINT"
echo "daemon-pi entrypoint contracts ok"
