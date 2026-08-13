#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${1:-.env.worktree}"

if [ -f "$ENV_FILE" ] && [ "${FORCE:-0}" != "1" ]; then
  echo "Refusing to overwrite existing $ENV_FILE. Re-run with FORCE=1 if you want to regenerate it."
  exit 1
fi

worktree_name="${WORKTREE_NAME:-$(basename "$PWD")}"
slug="$(printf '%s' "$worktree_name" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/_/g; s/__*/_/g; s/^_//; s/_$//')"
if [ -z "$slug" ]; then
  slug="dars"
fi

hash_value="$(printf '%s' "$PWD" | cksum | awk '{print $1}')"
offset=$((hash_value % 1000))

postgres_db="dars_lightweight_${slug}_${offset}"
postgres_port=5432
backend_port=$((18080 + offset))
frontend_port=$((13000 + offset))
frontend_origin="http://localhost:${frontend_port}"
agent_secret_key="$(openssl rand -base64 32)"

cat > "$ENV_FILE" <<EOF
POSTGRES_DB=${postgres_db}
POSTGRES_USER=dars
POSTGRES_PASSWORD=dars
POSTGRES_PORT=${postgres_port}
DATABASE_URL=postgres://dars:dars@localhost:${postgres_port}/${postgres_db}?sslmode=disable
EXPECTED_DATABASE_NAME=${postgres_db}
DARS_EDITION=lightweight
DATABASE_MAX_CONNS=25
DATABASE_MIN_CONNS=5
DATABASE_MAX_IDLE_CONNS=5
DATABASE_MAX_CONN_LIFETIME=300s
DARS_AGENT_SECRET_KEY=${agent_secret_key}
DARS_AGENT_SECRET_KEY_ID=primary

PORT=${backend_port}
JWT_SECRET=change-me-in-production
DARS_DEV_VERIFICATION_CODE=888888
DARS_SERVER_URL=ws://localhost:${backend_port}/ws
DARS_PUBLIC_URL=http://localhost:${backend_port}
DARS_APP_URL=${frontend_origin}

FRONTEND_PORT=${frontend_port}
FRONTEND_ORIGIN=${frontend_origin}
NEXT_PUBLIC_API_URL=http://localhost:${backend_port}
NEXT_PUBLIC_WS_URL=ws://localhost:${backend_port}/ws
EOF

echo "Generated $ENV_FILE for worktree '$worktree_name'"
echo "  Shared Postgres: localhost:${postgres_port}"
echo "  Database: ${postgres_db}"
echo "  Backend:  http://localhost:${backend_port}"
echo "  Frontend: ${frontend_origin}"
echo ""
echo "Next steps:"
echo "  make setup-worktree"
echo "  make start-worktree"
