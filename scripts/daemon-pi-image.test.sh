#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="${DARS_DAEMON_PI_IMAGE:-dars-daemon-pi:test}"
TEST_ROOT="$(mktemp -d)"
container_name="dars-daemon-pi-test-$$"
unhealthy_name="${container_name}-unhealthy"

cleanup() {
  docker rm -f "$container_name" "$unhealthy_name" >/dev/null 2>&1 || true
  rm -rf "$TEST_ROOT"
}
trap cleanup EXIT

cd "$ROOT_DIR"
if [ "${DARS_DAEMON_PI_SKIP_BUILD:-0}" != 1 ]; then
  docker build --progress=plain -f Dockerfile.daemon-pi -t "$IMAGE" .
fi

image_user="$(docker image inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$image_user" = dars ] || { echo "daemon-pi default user is $image_user, want dars"; exit 1; }

image_config="$(docker image inspect "$IMAGE" --format '{{json .Config}}')"
for expected in '"NODE_VERSION=22.19.0"' '"io.dars.pi.version":"0.84.2"' '"StartPeriod":120000000000' '"/usr/bin/tini","--","/usr/local/bin/daemon-pi-entrypoint"'; do
  grep -Fq "$expected" <<<"$image_config" || { echo "missing image contract: $expected"; exit 1; }
done
if grep -Eq 'DARS_(DAEMON_)?TOKEN=|ddt_[[:alnum:]_.-]+|/Users/logo/\.pi' <<<"$image_config"; then
  echo "image config contains a host path or credential"
  exit 1
fi

docker run --rm --entrypoint bash "$IMAGE" -c 'set -e; test "$(id -u)" != 0; test "$(node --version)" = v22.19.0; test "$(pi --version)" = 0.84.2; command -v dars git rg ssh tini >/dev/null; test ! -e /root/.pi; test ! -e /home/dars/.pi' >/dev/null

mkdir -p "$TEST_ROOT/config" "$TEST_ROOT/bin"
printf '{"defaultProvider":"xai","defaultModel":"grok-4.6"}\n' >"$TEST_ROOT/config/settings.json"
printf 'ddt_image_fixture\n' >"$TEST_ROOT/token"
cp scripts/testdata/daemon-pi/fake-dars "$TEST_ROOT/bin/dars"
chmod +x "$TEST_ROOT/bin/dars"

common_args=(
  --env "PATH=/testbin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
  --env "DARS_SERVER_URL=http://dars.test"
  --env "DARS_WORKSPACE_ID=10000000-0000-0000-0000-000000000001"
  --env "DARS_DAEMON_ID=daemon-pi-image-test"
  --env "DARS_DAEMON_TOKEN_FILE=/run/secrets/dars_daemon_token"
  --env "DARS_PI_MODEL=xai/grok-4.6"
  --env "DARS_PI_PREFLIGHT=disabled"
  --env "FAKE_DAEMON_PI_LOG=/tmp/daemon-pi-calls.log"
  --env "FAKE_DARS_STAY_RUNNING=1"
  --mount "type=bind,src=$TEST_ROOT/bin,dst=/testbin,readonly"
  --mount "type=bind,src=$TEST_ROOT/config,dst=/config/pi,readonly"
  --mount "type=bind,src=$TEST_ROOT/token,dst=/run/secrets/dars_daemon_token,readonly"
  --health-start-period=1s
  --health-interval=1s
  --health-timeout=1s
  --health-retries=2
)

wait_for_health() {
  local name="$1" wanted="$2"
  for _ in {1..30}; do
    current="$(docker inspect "$name" --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}')"
    if [ "$current" = "$wanted" ]; then
      return
    fi
    sleep 1
  done
  docker inspect "$name" --format '{{json .State}}' >&2
  echo "container $name did not become $wanted" >&2
  return 1
}

docker run --detach --name "$container_name" "${common_args[@]}" "$IMAGE" >/dev/null
wait_for_health "$container_name" healthy
pid1="$(docker exec "$container_name" bash -c 'tr "\0" " " </proc/1/cmdline')"
[[ "$pid1" == "/usr/bin/tini -- /usr/local/bin/daemon-pi-entrypoint " ]] || { echo "unexpected PID 1: $pid1"; exit 1; }
docker stop --time 5 "$container_name" >/dev/null
docker rm "$container_name" >/dev/null

docker run --detach --name "$unhealthy_name" "${common_args[@]}" --env FAKE_DARS_HEALTH_FAIL=1 "$IMAGE" >/dev/null
wait_for_health "$unhealthy_name" unhealthy
docker rm -f "$unhealthy_name" >/dev/null

failure_args=(
  --env "DARS_SERVER_URL=http://127.0.0.1:9"
  --env "DARS_WORKSPACE_ID=10000000-0000-0000-0000-000000000001"
  --env "DARS_DAEMON_ID=daemon-pi-image-test"
  --env "DARS_DAEMON_TOKEN_FILE=/run/secrets/dars_daemon_token"
  --env "DARS_PI_MODEL=xai/grok-4.6"
  --env "DARS_PI_PREFLIGHT=disabled"
  --mount "type=bind,src=$TEST_ROOT/config,dst=/config/pi,readonly"
  --mount "type=bind,src=$TEST_ROOT/token,dst=/run/secrets/dars_daemon_token,readonly"
)
if docker run --rm "${failure_args[@]}" "$IMAGE" >/dev/null 2>&1; then
  echo "image unexpectedly started with unreachable DARS Server"
  exit 1
fi

digest="$(docker image inspect "$IMAGE" --format '{{.Id}}')"
printf 'daemon-pi image contracts ok: %s\n' "$digest"
