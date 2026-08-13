#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHART_DIR="$ROOT_DIR/deploy/helm/dars"

require_rendered_value() {
  local rendered=$1
  local expected=$2

  if ! grep -Fq "$expected" <<<"$rendered"; then
    echo "Missing expected Helm-rendered config value:"
    echo "  $expected"
    exit 1
  fi
}

helm lint "$CHART_DIR"

default_config="$(
  helm template dars "$CHART_DIR" \
    --show-only templates/configmap.yaml
)"
require_rendered_value "$default_config" 'DARS_EDITION: "lightweight"'
require_rendered_value "$default_config" 'EXPECTED_DATABASE_NAME: "dars_lightweight"'
require_rendered_value "$default_config" 'DATABASE_MAX_CONNS: "25"'
require_rendered_value "$default_config" 'DATABASE_MIN_CONNS: "5"'
require_rendered_value "$default_config" 'DATABASE_MAX_IDLE_CONNS: "5"'
require_rendered_value "$default_config" 'DATABASE_MAX_CONN_LIFETIME: "300s"'

default_backend="$(
  helm template dars "$CHART_DIR" \
    --show-only templates/backend.yaml
)"
require_rendered_value "$default_backend" 'path: /readyz'
require_rendered_value "$default_backend" 'path: /health'
if grep -Fq 'path: /healthz' <<<"$default_backend"; then
  echo "Helm backend still references removed /healthz route"
  exit 1
fi

if grep -Eq 'VCS|GOOGLE|S3_|CLOUDFRONT|UPLOAD' <<<"$default_config"; then
  echo "Helm config still renders a retired capability"
  exit 1
fi

echo "helm config rendering ok"
