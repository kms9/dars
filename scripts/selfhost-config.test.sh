#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_line() {
  local output=$1
  local expected=$2
  if ! grep -Fxq "$expected" <<<"$output"; then
    echo "Missing expected value: $expected"
    echo "$output"
    exit 1
  fi
}

tmp_env="$(mktemp)"
tmp_dir="$(mktemp -d)"
trap 'rm -f "$tmp_env"; rm -rf "$tmp_dir"' EXIT

sed \
  -e 's/^PORT=.*/PORT=9100/' \
  -e 's/^FRONTEND_PORT=.*/FRONTEND_PORT=3100/' \
  -e '/^DARS_SERVER_URL=/d' \
  .env.example >"$tmp_env"

config="$(
  docker compose \
    --env-file "$tmp_env" \
    -f docker-compose.selfhost.yml \
    config
)"

for expected in \
  'published: "3100"' \
  'published: "9100"' \
  'POSTGRES_DB: dars_lightweight' \
  'EXPECTED_DATABASE_NAME: dars_lightweight' \
  'DARS_EDITION: lightweight' \
  'FRONTEND_ORIGIN: http://localhost:3100'; do
  if ! grep -Fq "$expected" <<<"$config"; then
    echo "Missing expected Docker Compose value: $expected"
    exit 1
  fi
done

if grep -Fq 'GOOGLE_REDIRECT_URI' <<<"$config"; then
  echo "Google OAuth configuration must not re-enter the Lightweight stack."
  exit 1
fi

local_env="$(
  env -i PATH="$PATH" bash -c '
    set -euo pipefail
    env_file=$1
    set -a
    # shellcheck disable=SC1090
    . "$env_file"
    set +a
    # shellcheck disable=SC1091
    . scripts/local-env.sh
    printf "%s\n" \
      "PORT=${PORT}" \
      "FRONTEND_PORT=${FRONTEND_PORT}" \
      "FRONTEND_ORIGIN=${FRONTEND_ORIGIN}" \
      "DARS_APP_URL=${DARS_APP_URL}" \
      "DARS_SERVER_URL=${DARS_SERVER_URL}" \
      "LOCAL_UPLOAD_BASE_URL=${LOCAL_UPLOAD_BASE_URL}" \
      "PLAYWRIGHT_BASE_URL=${PLAYWRIGHT_BASE_URL}"
  ' _ "$tmp_env"
)"

require_line "$local_env" 'PORT=9100'
require_line "$local_env" 'FRONTEND_PORT=3100'
require_line "$local_env" 'FRONTEND_ORIGIN=http://localhost:3100'
require_line "$local_env" 'DARS_APP_URL=http://localhost:3100'
require_line "$local_env" 'DARS_SERVER_URL=ws://localhost:9100/ws'
require_line "$local_env" 'LOCAL_UPLOAD_BASE_URL=http://localhost:9100'
require_line "$local_env" 'PLAYWRIGHT_BASE_URL=http://localhost:3100'

worktree_env="$tmp_dir/.env.worktree"
WORKTREE_NAME=selfhost-config-test bash scripts/init-worktree-env.sh "$worktree_env" >/dev/null
if grep -Eq '^GOOGLE_(CLIENT_ID|CLIENT_SECRET|REDIRECT_URI)=' "$worktree_env"; then
  echo "Generated Lightweight worktree env must not contain Google OAuth variables."
  exit 1
fi
grep -Fxq 'DARS_EDITION=lightweight' "$worktree_env"

for script in scripts/dev.sh scripts/check.sh; do
  grep -Fq '. scripts/local-env.sh' "$script"
done

bash -n scripts/local-env.sh scripts/init-worktree-env.sh
echo "self-host Lightweight configuration ok"
