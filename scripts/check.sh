#!/usr/bin/env bash
set -euo pipefail

# ==========================================================================
# Full target verification pipeline on a disposable Lightweight database:
# typecheck → build → unit/contract tests → Go tests → browser E2E
# Usage: bash scripts/check.sh
# ==========================================================================

ENV_FILE="${ENV_FILE:-.env}"
if [ ! -f "$ENV_FILE" ]; then
  echo "Missing env file: $ENV_FILE"
  echo "Create .env from .env.example, or run 'make worktree-env' and use .env.worktree."
  exit 1
fi

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

# shellcheck disable=SC1091
. scripts/local-env.sh

# Keep the disposable E2E services independent from an already-running local
# app without mutating the checkout's .env. These overrides intentionally apply
# after loading the env file, unlike the normal runtime port variables.
validate_check_port() {
  local value=$1 name=$2
  if [[ ! "$value" =~ ^[0-9]+$ ]] || [ "$value" -lt 1 ] || [ "$value" -gt 65535 ]; then
    echo "Invalid $name: $value"
    exit 1
  fi
}

if [ -n "${CHECK_BACKEND_PORT:-}" ]; then
  validate_check_port "$CHECK_BACKEND_PORT" CHECK_BACKEND_PORT
  PORT="$CHECK_BACKEND_PORT"
  BACKEND_PORT="$CHECK_BACKEND_PORT"
  DARS_PUBLIC_URL="http://localhost:${CHECK_BACKEND_PORT}"
fi
if [ -n "${CHECK_FRONTEND_PORT:-}" ]; then
  validate_check_port "$CHECK_FRONTEND_PORT" CHECK_FRONTEND_PORT
  FRONTEND_PORT="$CHECK_FRONTEND_PORT"
  FRONTEND_ORIGIN="http://localhost:${CHECK_FRONTEND_PORT}"
  DARS_APP_URL="$FRONTEND_ORIGIN"
  PLAYWRIGHT_BASE_URL="$FRONTEND_ORIGIN"
fi
export PORT BACKEND_PORT FRONTEND_PORT FRONTEND_ORIGIN DARS_APP_URL DARS_PUBLIC_URL PLAYWRIGHT_BASE_URL

SOURCE_DATABASE_URL="${DATABASE_URL:-postgres://${POSTGRES_USER:-dars}:${POSTGRES_PASSWORD:-dars}@localhost:${POSTGRES_PORT:-5432}/${POSTGRES_DB:-dars_lightweight}?sslmode=disable}"
SOURCE_DATABASE_NO_QUERY="${SOURCE_DATABASE_URL%%\?*}"
SOURCE_DATABASE_QUERY=""
if [[ "$SOURCE_DATABASE_URL" == *\?* ]]; then
  SOURCE_DATABASE_QUERY="?${SOURCE_DATABASE_URL#*\?}"
fi

case "$SOURCE_DATABASE_NO_QUERY" in
  postgres://*|postgresql://*) ;;
  *)
    echo "Unsupported DATABASE_URL for isolated check database."
    exit 1
    ;;
esac

database_authority="${SOURCE_DATABASE_NO_QUERY%/*}"
database_location="${SOURCE_DATABASE_NO_QUERY#*://}"
database_authority_without_scheme="${database_location%%/*}"
database_hostport="${database_authority_without_scheme##*@}"
database_port="${POSTGRES_PORT:-5432}"
if [[ "$database_hostport" == \[* ]]; then
  database_host="${database_hostport#\[}"
  database_host="${database_host%%]*}"
  database_port_part="${database_hostport#*\]}"
  if [[ "$database_port_part" == :* ]] && [ -n "${database_port_part#:}" ]; then
    database_port="${database_port_part#:}"
  fi
else
  database_host="${database_hostport%%:*}"
  if [[ "$database_hostport" == *:* ]] && [ -n "${database_hostport##*:}" ]; then
    database_port="${database_hostport##*:}"
  fi
fi

case "$database_host" in
  localhost|127.*|::1) ;;
  *)
    echo "Refusing to create a check database on non-loopback host: $database_host"
    echo "Run make check against a local disposable PostgreSQL instance."
    exit 1
    ;;
esac

check_suffix="${CHECK_DATABASE_SUFFIX:-${GITHUB_RUN_ID:-local}_${GITHUB_RUN_ATTEMPT:-0}_$$}"
check_suffix="$(printf '%s' "$check_suffix" | tr '[:upper:].-' '[:lower:]__' | tr -cd 'a-z0-9_' | cut -c1-31)"
CHECK_DATABASE_NAME="${CHECK_DATABASE_NAME:-dars_lightweight_check_${check_suffix}}"
if [[ ! "$CHECK_DATABASE_NAME" =~ ^dars_lightweight_check_[a-z0-9_]+$ ]]; then
  echo "Refusing unsafe check database name: $CHECK_DATABASE_NAME"
  exit 1
fi

CHECK_DATABASE_URL="${database_authority}/${CHECK_DATABASE_NAME}${SOURCE_DATABASE_QUERY}"

BACKEND_PID=""
FRONTEND_PID=""
BACKEND_PGID=""
FRONTEND_PGID=""
CHECK_PGID="$(ps -o pgid= -p $$ | tr -d ' ')"
STARTED_BACKEND=false
STARTED_FRONTEND=false
FRESH_DATABASE_CREATED=false
EXIT_CODE=0

run_admin_psql() {
  PGHOST="$database_host" \
    PGPORT="$database_port" \
    PGUSER="${POSTGRES_USER:-dars}" \
    PGPASSWORD="${POSTGRES_PASSWORD:-dars}" \
    PGDATABASE=postgres \
    psql -X --set ON_ERROR_STOP=1 --no-psqlrc "$@"
}

# --------------------------------------------------------------------------
# Cleanup: kill only services this script started
# --------------------------------------------------------------------------
stop_service() {
  local pid=$1 process_group=$2

  # pnpm and `go run` both create child processes. Killing only the wrapper
  # can reparent Next/server to PID 1 and leave the check ports occupied. The
  # launch sites below enable job control for one command so every descendant
  # inherits a dedicated process group; terminate that group as one unit.
  if [ -n "$process_group" ] && [ "$process_group" != "$CHECK_PGID" ]; then
    kill -TERM -- "-$process_group" 2>/dev/null || true
    for _ in {1..50}; do
      if ! kill -0 -- "-$process_group" 2>/dev/null; then
        break
      fi
      sleep 0.1
    done
    kill -KILL -- "-$process_group" 2>/dev/null || true
  else
    kill -TERM "$pid" 2>/dev/null || true
  fi
  wait "$pid" 2>/dev/null || true
}

cleanup() {
  local command_status=$?
  trap - EXIT
  if [ "$EXIT_CODE" -eq 0 ] && [ "$command_status" -ne 0 ]; then
    EXIT_CODE=$command_status
  fi
  echo ""
  if [ "$STARTED_BACKEND" = true ] && [ -n "$BACKEND_PID" ]; then
    stop_service "$BACKEND_PID" "$BACKEND_PGID"
    echo "    Stopped backend (PID $BACKEND_PID)"
  fi
  if [ "$STARTED_FRONTEND" = true ] && [ -n "$FRONTEND_PID" ]; then
    stop_service "$FRONTEND_PID" "$FRONTEND_PGID"
    echo "    Stopped frontend (PID $FRONTEND_PID)"
  fi
  if [ "$FRESH_DATABASE_CREATED" = true ]; then
    run_admin_psql --set check_database_name="$CHECK_DATABASE_NAME" >/dev/null <<'SQL' || true
DROP DATABASE IF EXISTS :"check_database_name" WITH (FORCE);
SQL
    echo "    Dropped fresh check database '$CHECK_DATABASE_NAME'"
  fi
  echo ""
  if [ "$EXIT_CODE" -eq 0 ]; then
    echo "✓ All checks passed."
  else
    echo "✗ Checks FAILED."
  fi
  exit "$EXIT_CODE"
}
trap cleanup EXIT

# --------------------------------------------------------------------------
# Utility: wait until a port responds
# --------------------------------------------------------------------------
wait_for_port() {
  local port=$1 name=$2 max_wait=${3:-60} path=${4:-/}
  local elapsed=0
  echo "    Waiting for $name on :$port..."
  while ! curl -sf "http://localhost:${port}${path}" > /dev/null 2>&1; do
    sleep 1
    elapsed=$((elapsed + 1))
    if [ "$elapsed" -ge "$max_wait" ]; then
      echo "    ERROR: $name did not start within ${max_wait}s"
      EXIT_CODE=1
      exit 1
    fi
  done
  echo "    $name ready (${elapsed}s)"
}

# --------------------------------------------------------------------------
# Step 0: Create an isolated fresh database
# --------------------------------------------------------------------------
echo "==> Using env file: $ENV_FILE"
echo "==> Checking PostgreSQL..."
bash scripts/ensure-postgres.sh "$ENV_FILE"
echo "==> Creating fresh check database '$CHECK_DATABASE_NAME'..."
run_admin_psql --set check_database_name="$CHECK_DATABASE_NAME" >/dev/null <<'SQL'
DROP DATABASE IF EXISTS :"check_database_name" WITH (FORCE);
CREATE DATABASE :"check_database_name";
SQL
FRESH_DATABASE_CREATED=true

export DATABASE_URL="$CHECK_DATABASE_URL"
export LIGHTWEIGHT_DATABASE_URL="$CHECK_DATABASE_URL"
export POSTGRES_DB="$CHECK_DATABASE_NAME"
export EXPECTED_DATABASE_NAME="$CHECK_DATABASE_NAME"
export DARS_EDITION=lightweight
export APP_ENV="${APP_ENV:-development}"
export DARS_DEV_VERIFICATION_CODE="${DARS_DEV_VERIFICATION_CODE:-424242}"
export DARS_AGENT_SECRET_KEY="${DARS_AGENT_SECRET_KEY:-$(openssl rand -base64 32)}"
export DARS_AGENT_SECRET_KEY_ID="${DARS_AGENT_SECRET_KEY_ID:-check}"
# Build and serve Web with a same-origin proxy pinned to this check run's
# dedicated backend. This must be set before `pnpm build` because production
# rewrites are materialized into the Next.js build output.
export REMOTE_API_URL="http://localhost:${PORT}"
export NEXT_PUBLIC_API_URL="http://localhost:${FRONTEND_PORT}"
export NEXT_PUBLIC_WS_URL="ws://localhost:${FRONTEND_PORT}/ws"
echo "✓ Fresh check database ready: $CHECK_DATABASE_NAME"

# --------------------------------------------------------------------------
# Step 1: TypeScript typecheck
# --------------------------------------------------------------------------
echo ""
echo "==> [1/6] Target TypeScript typecheck..."
pnpm typecheck || { EXIT_CODE=1; exit 1; }

# --------------------------------------------------------------------------
# Step 2: Target Web production build
# --------------------------------------------------------------------------
echo ""
echo "==> [2/6] Target Web production build..."
pnpm build || { EXIT_CODE=1; exit 1; }

# --------------------------------------------------------------------------
# Step 3: Target TypeScript unit and Web contract tests
# --------------------------------------------------------------------------
echo ""
echo "==> [3/6] Target TypeScript unit and Web contract tests..."
pnpm test || { EXIT_CODE=1; exit 1; }

# --------------------------------------------------------------------------
# Step 4: Go tests
# --------------------------------------------------------------------------
echo ""
echo "==> [4/6] Go tests against the fresh check database..."
echo "==> Verifying Go test wrapper..."
bash scripts/test-go.test.sh || { EXIT_CODE=1; exit 1; }
echo "==> Running database migrations..."
(cd server && go run ./cmd/migrate up) || { EXIT_CODE=1; exit 1; }
bash scripts/test-go.sh || { EXIT_CODE=1; exit 1; }

# --------------------------------------------------------------------------
# Step 5: Start dedicated services for browser E2E
# --------------------------------------------------------------------------
echo ""
echo "==> [5/6] Starting dedicated services for browser E2E..."

if curl -sf "http://localhost:${PORT}/health" > /dev/null 2>&1; then
  echo "    ERROR: backend port $PORT is already serving an app."
  echo "    Stop it or select an unused PORT; make check never reuses a process connected to another database."
  EXIT_CODE=1
  exit 1
fi

echo "    Starting backend..."
set -m
(cd server && exec go run ./cmd/server) > /tmp/dars-check-backend.log 2>&1 &
BACKEND_PID=$!
set +m
BACKEND_PGID="$(ps -o pgid= -p "$BACKEND_PID" | tr -d ' ')"
STARTED_BACKEND=true
wait_for_port "$PORT" "Backend" 90 "/readyz"

if curl -sf "http://localhost:${FRONTEND_PORT}" > /dev/null 2>&1; then
  echo "    ERROR: frontend port $FRONTEND_PORT is already serving an app."
  echo "    Stop it or select an unused FRONTEND_PORT; make check requires its own Web process."
  EXIT_CODE=1
  exit 1
fi

echo "    Starting frontend..."
set -m
pnpm --filter @dars/web exec next start --port "$FRONTEND_PORT" > /tmp/dars-check-frontend.log 2>&1 &
FRONTEND_PID=$!
set +m
FRONTEND_PGID="$(ps -o pgid= -p "$FRONTEND_PID" | tr -d ' ')"
STARTED_FRONTEND=true
wait_for_port "$FRONTEND_PORT" "Frontend" 120 "/"

# --------------------------------------------------------------------------
# Step 6: Browser E2E tests (Playwright)
# --------------------------------------------------------------------------
echo ""
echo "==> [6/6] Browser E2E tests (Playwright)..."
pnpm exec playwright test || { EXIT_CODE=1; exit 1; }
