#!/usr/bin/env bash
# Idempotent repository bootstrap for the DARS Cloud Agent environment.
#
# System toolchains (Go 1.26+, Docker + fuse-overlayfs, PostgreSQL client, jq)
# are provided by the environment base image/snapshot. This script only prepares
# repository-derived state and must remain safe to run repeatedly.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# ---------- Local dev env file ----------
# .env is gitignored. Create it once with local-only development defaults.
if [ ! -f .env ]; then
  echo "==> Creating .env from .env.example..."
  cp .env.example .env
  JWT="$(openssl rand -hex 32)"
  AGENTKEY="$(openssl rand -base64 32)"
  sed -i "s/^JWT_SECRET=.*/JWT_SECRET=${JWT}/" .env
  sed -i "s#^DARS_AGENT_SECRET_KEY=.*#DARS_AGENT_SECRET_KEY=${AGENTKEY}#" .env
  # Fixed dev verification code enables deterministic local sign-in as
  # dev@local.test (see .env.example and CONTRIBUTING.md).
  sed -i "s/^DARS_DEV_VERIFICATION_CODE=.*/DARS_DEV_VERIFICATION_CODE=424242/" .env
fi

# ---------- JavaScript dependencies ----------
corepack enable >/dev/null 2>&1 || true
echo "==> Installing JS dependencies (pnpm)..."
pnpm install --frozen-lockfile

# ---------- Go module cache + binaries ----------
echo "==> Warming Go module cache and building server binaries..."
(
  cd server
  go mod download
  go build -o bin/server ./cmd/server
  go build -o bin/dars ./cmd/dars
  go build -o bin/migrate ./cmd/migrate
)

echo "✓ Install complete."
