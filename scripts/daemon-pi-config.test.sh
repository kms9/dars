#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
TEST_ROOT="$(mktemp -d)"
trap 'rm -rf "$TEST_ROOT"' EXIT

mkdir -p "$TEST_ROOT/pi"
printf 'ddt_fixture\n' >"$TEST_ROOT/token"
config="$(
  env DARS_SERVER_URL=http://host.docker.internal:8080 DARS_WORKSPACE_ID=10000000-0000-0000-0000-000000000001 DARS_DAEMON_ID=daemon-pi-test DARS_PI_CONFIG_DIR="$TEST_ROOT/pi" DARS_DAEMON_TOKEN_FILE=/run/secrets/dars_daemon_token DARS_DAEMON_TOKEN_SECRET_FILE="$TEST_ROOT/token" docker compose -f docker-compose.daemon-pi.yml config
)"

expected_contracts=(
  'dockerfile: Dockerfile.daemon-pi'
  'DARS_DAEMON_TOKEN_FILE: /run/secrets/dars_daemon_token'
  'target: /config/pi'
  'target: /home/dars/.dars'
  'target: /workspaces'
  'host.docker.internal=host-gateway'
)
for expected in "${expected_contracts[@]}"; do
  grep -Fq "$expected" <<<"$config" || { echo "missing Compose contract: $expected"; exit 1; }
done

env_config="$(
  env -u DARS_DAEMON_TOKEN_FILE DARS_SERVER_URL=http://host.docker.internal:8080 DARS_WORKSPACE_ID=10000000-0000-0000-0000-000000000001 DARS_DAEMON_ID=daemon-pi-test DARS_PI_CONFIG_DIR="$TEST_ROOT/pi" DARS_DAEMON_TOKEN=ddt_fixture docker compose -f docker-compose.daemon-pi.yml config
)"
grep -Fq 'DARS_DAEMON_TOKEN: ddt_fixture' <<<"$env_config"
grep -Fq 'DARS_DAEMON_TOKEN_FILE: null' <<<"$env_config"

if grep -Eq 'docker\.sock|target: /home/dars$' <<<"$config"; then
  echo "Compose mounts an unsafe host resource"
  exit 1
fi

grep -Fq 'FROM node:22.19.0-bookworm-slim' Dockerfile.daemon-pi
grep -Fq '@earendil-works/pi-coding-agent@${PI_VERSION}' Dockerfile.daemon-pi
grep -Fq 'USER dars' Dockerfile.daemon-pi
grep -Fq 'HEALTHCHECK --interval=15s --timeout=5s --start-period=120s' Dockerfile.daemon-pi
grep -Fq 'ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/daemon-pi-entrypoint"]' Dockerfile.daemon-pi
bash -n docker/daemon-pi-entrypoint.sh
echo "daemon-pi image and Compose contracts ok"
