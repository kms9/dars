#!/usr/bin/env bash
# Per-boot startup for the DARS Cloud Agent environment.
#
# Brings up the Docker daemon (this nested VM has no systemd) and the shared
# PostgreSQL (pgvector) container, applies database migrations, then launches
# the Go backend and the Next.js web frontend and stays attached to them.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# ---------- Docker daemon ----------
# The base image uses the fuse-overlayfs storage driver (configured in
# /etc/docker/daemon.json) because the classic overlayfs driver cannot extract
# image whiteout files inside this nested container.
if ! sudo docker info >/dev/null 2>&1; then
  echo "==> Starting dockerd..."
  sudo rm -f /var/run/docker.pid
  sudo nohup dockerd >/tmp/dockerd.log 2>&1 &
  for _ in $(seq 1 30); do
    if sudo docker info >/dev/null 2>&1; then break; fi
    sleep 1
  done
fi
sudo docker info >/dev/null 2>&1 || { echo "dockerd failed to start"; tail -20 /tmp/dockerd.log || true; exit 1; }
# Allow the repo's docker/compose invocations (which do not use sudo) to reach the
# daemon without depending on the login shell picking up the docker group.
sudo chmod 666 /var/run/docker.sock 2>/dev/null || true
echo "Docker daemon is running."

# ---------- PostgreSQL (pgvector) ----------
# Starts the pgvector/pgvector:pg17 container via docker compose and ensures the
# target database exists.
bash scripts/ensure-postgres.sh .env

# ---------- Load env ----------
set -a
# shellcheck disable=SC1091
. ./.env
set +a

# ---------- Database migrations ----------
echo "==> Running database migrations..."
(cd server && go run ./cmd/migrate up)

# ---------- Application services ----------
echo "==> Starting backend (:${PORT:-8080}) and web frontend (:${FRONTEND_PORT:-3000})..."
trap 'kill 0' EXIT
if [ -x server/bin/server ]; then
  (cd server && exec ./bin/server) &
else
  (cd server && exec go run ./cmd/server) &
fi
pnpm dev:web &
wait
