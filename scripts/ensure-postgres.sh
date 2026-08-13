#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${1:-.env}"

if [ ! -f "$ENV_FILE" ]; then
  echo "Missing env file: $ENV_FILE"
  echo "Create .env from .env.example, or run 'make worktree-env' and use .env.worktree."
  exit 1
fi

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

POSTGRES_DB="${POSTGRES_DB:-dars_lightweight}"
POSTGRES_USER="${POSTGRES_USER:-dars}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-dars}"
DATABASE_URL="${DATABASE_URL:-}"
EXPECTED_DATABASE_NAME="${EXPECTED_DATABASE_NAME:-$POSTGRES_DB}"

export PGPASSWORD="$POSTGRES_PASSWORD"

db_host=""
db_port="${POSTGRES_PORT:-5432}"
db_name="$POSTGRES_DB"

parse_database_url() {
  local rest authority hostport path port_part

  rest="${DATABASE_URL#*://}"
  rest="${rest%%\?*}"
  authority="${rest%%/*}"
  path="${rest#*/}"

  if [ "$authority" = "$rest" ]; then
    path=""
  fi

  hostport="${authority##*@}"

  if [[ "$hostport" == \[* ]]; then
    db_host="${hostport#\[}"
    db_host="${db_host%%]*}"
    port_part="${hostport#*\]}"
    if [[ "$port_part" == :* ]] && [ -n "${port_part#:}" ]; then
      db_port="${port_part#:}"
    fi
  else
    db_host="${hostport%%:*}"
    if [[ "$hostport" == *:* ]] && [ -n "${hostport##*:}" ]; then
      db_port="${hostport##*:}"
    fi
  fi

  if [ -n "$path" ]; then
    db_name="${path%%/*}"
  fi
}

if [ -n "$DATABASE_URL" ]; then
  parse_database_url
fi

if [[ ! "$db_name" =~ ^dars_lightweight(_[a-z0-9_]+)?$ ]]; then
  echo "Refusing database outside the Lightweight name allowlist: $db_name"
  exit 1
fi
if [ "$EXPECTED_DATABASE_NAME" != "$db_name" ]; then
  echo "Refusing database whose URL name does not match EXPECTED_DATABASE_NAME."
  exit 1
fi

is_local() {
  [ -z "$DATABASE_URL" ] || [ "$db_host" = "localhost" ] || [ "$db_host" = "127.0.0.1" ] || [ "$db_host" = "::1" ]
}

if is_local; then
  # ---------- Local: use Docker ----------
  echo "==> Ensuring shared PostgreSQL container is running on localhost:5432..."
  docker compose up -d postgres

  echo "==> Waiting for PostgreSQL to be ready..."
  until docker compose exec -T postgres pg_isready -U "$POSTGRES_USER" -d postgres > /dev/null 2>&1; do
    sleep 1
  done

  echo "==> Ensuring database '$db_name' exists..."
  db_exists="$(docker compose exec -T postgres \
    psql -U "$POSTGRES_USER" -d postgres -Atqc "SELECT 1 FROM pg_database WHERE datname = '$db_name'")"

  if [ "$db_exists" != "1" ]; then
    docker compose exec -T postgres \
      psql -U "$POSTGRES_USER" -d postgres -v ON_ERROR_STOP=1 \
      -c "CREATE DATABASE \"$db_name\"" \
      > /dev/null
  fi

  echo "✓ PostgreSQL ready (local Docker). Database: $db_name"
else
  # ---------- Remote: skip Docker, verify connectivity ----------
  echo "==> Remote database detected (host: $db_host). Skipping Docker."
  if command -v psql > /dev/null 2>&1; then
    echo "==> Verifying PostgreSQL authentication and database access at $db_host:$db_port..."
    PGCONNECT_TIMEOUT=5 psql -X --no-psqlrc --set ON_ERROR_STOP=1 \
      --host "$db_host" --port "$db_port" --username "$POSTGRES_USER" \
      --dbname "$db_name" --tuples-only --no-align \
      --command "SELECT 1" > /dev/null
    echo "✓ PostgreSQL authenticated (remote: $db_host:$db_port). Database: $db_name"
  elif command -v pg_isready > /dev/null 2>&1; then
    echo "==> psql not found; checking server readiness only at $db_host:$db_port..."
    pg_isready --host "$db_host" --port "$db_port" --username "$POSTGRES_USER" \
      --dbname "$db_name" > /dev/null
    echo "✓ PostgreSQL server reachable; authentication was not verified. Database: $db_name"
  else
    echo "Neither psql nor pg_isready is installed; cannot verify the remote database."
    exit 1
  fi
fi
