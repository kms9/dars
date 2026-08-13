# Shared local development env derivation. Source this after loading .env.

POSTGRES_DB="${POSTGRES_DB:-dars_lightweight}"
POSTGRES_USER="${POSTGRES_USER:-dars}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
EXPECTED_DATABASE_NAME="${EXPECTED_DATABASE_NAME:-${POSTGRES_DB}}"
DARS_EDITION="${DARS_EDITION:-lightweight}"

PORT="${BACKEND_PORT:-${API_PORT:-${SERVER_PORT:-${PORT:-8080}}}}"
FRONTEND_PORT="${FRONTEND_PORT:-3000}"
FRONTEND_ORIGIN="${FRONTEND_ORIGIN:-http://localhost:${FRONTEND_PORT}}"

# Older generated worktree env files predate DARS_PUBLIC_URL. Derive it
# only when the variable is absent; an explicitly configured value, including
# an intentionally empty one for same-origin proxying, must be preserved.
if [ "${DARS_PUBLIC_URL+x}" != "x" ]; then
  DARS_PUBLIC_URL="http://localhost:${PORT}"
fi
DARS_APP_URL="${DARS_APP_URL:-${FRONTEND_ORIGIN}}"
# Keep the checked-in localhost default aligned with a custom backend port.
# Preserve any explicitly configured non-default server URL.
if [ "${DARS_SERVER_URL:-}" = "" ] || [ "$DARS_SERVER_URL" = "ws://localhost:8080/ws" ]; then
  DARS_SERVER_URL="ws://localhost:${PORT}/ws"
fi
LOCAL_UPLOAD_BASE_URL="${LOCAL_UPLOAD_BASE_URL:-http://localhost:${PORT}}"
PLAYWRIGHT_BASE_URL="${PLAYWRIGHT_BASE_URL:-${FRONTEND_ORIGIN}}"

export POSTGRES_DB POSTGRES_USER POSTGRES_PORT EXPECTED_DATABASE_NAME DARS_EDITION
export PORT FRONTEND_PORT FRONTEND_ORIGIN
export DARS_PUBLIC_URL DARS_APP_URL DARS_SERVER_URL LOCAL_UPLOAD_BASE_URL
export PLAYWRIGHT_BASE_URL
