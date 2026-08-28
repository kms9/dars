#!/usr/bin/env bash
set -euo pipefail

log() {
  printf '[daemon-pi] %s\n' "$*" >&2
}

fail() {
  log "error: $*"
  exit 1
}

require_value() {
  local name="$1"
  [ -n "${!name:-}" ] || fail "$name is required"
}

read_daemon_token() {
  local token
  if [ "${DARS_DAEMON_TOKEN_FILE+x}" = x ]; then
    [ -n "${DARS_DAEMON_TOKEN_FILE}" ] || fail "DARS_DAEMON_TOKEN_FILE is set but empty"
    [ -r "${DARS_DAEMON_TOKEN_FILE}" ] || fail "DARS_DAEMON_TOKEN_FILE is not readable"
    token="$(tr -d '\r\n' <"${DARS_DAEMON_TOKEN_FILE}")"
    [[ "$token" == ddt_* ]] || fail "DARS_DAEMON_TOKEN_FILE must contain a ddt_ Daemon Token"
    return
  fi
  [ -n "${DARS_DAEMON_TOKEN:-}" ] || fail "DARS_DAEMON_TOKEN_FILE or DARS_DAEMON_TOKEN is required"
  [[ "${DARS_DAEMON_TOKEN}" == ddt_* ]] || fail "DARS_DAEMON_TOKEN must contain a ddt_ Daemon Token"
}

resolve_pi_model() {
  local requested="${DARS_PI_MODEL:-}"
  if [ -z "$requested" ]; then
    requested="$(node -e '
      const fs = require("fs");
      const path = require("path");
      const file = path.join(process.env.PI_CODING_AGENT_DIR, "settings.json");
      const settings = JSON.parse(fs.readFileSync(file, "utf8"));
      if (!settings.defaultProvider || !settings.defaultModel) process.exit(2);
      process.stdout.write(`${settings.defaultProvider}/${settings.defaultModel}`);
    ')" || fail "could not resolve defaultProvider/defaultModel from Pi settings.json"
  fi
  [[ "$requested" == */* ]] || fail "DARS_PI_MODEL must use provider/model form"
  PI_PROVIDER="${requested%%/*}"
  PI_MODEL="${requested#*/}"
  [ -n "$PI_PROVIDER" ] && [ -n "$PI_MODEL" ] || fail "DARS_PI_MODEL must use provider/model form"
  export DARS_PI_MODEL="${PI_PROVIDER}/${PI_MODEL}"
}

pi_catalog_contains_model() {
  local catalog="$1"
  awk -v provider="$PI_PROVIDER" -v model="$PI_MODEL" '
    NR > 1 && $1 == provider && $2 == model { found = 1 }
    END { exit found ? 0 : 1 }
  ' "$catalog"
}

pi_auth_check() {
  local output_file="$1"
  shift
  "${DARS_PI_PATH}" auth check --model "${PI_PROVIDER}/${PI_MODEL}" --json "$@" >"$output_file" 2>/dev/null || true
  PI_AUTH_STATE="$(node -e '
    const fs = require("fs");
    try {
      const result = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
      if (result.status === "ready") process.stdout.write("ready");
      else if (result.status === "invalid" && String(result.provider || "").includes("/")) process.stdout.write("custom");
      else process.stdout.write("not_ready");
    } catch {
      process.stdout.write("not_ready");
    }
  ' "$output_file")"
}

run_pi_preflight() {
  local mode="${DARS_PI_PREFLIGHT:-basic}"
  case "$mode" in
    basic|active|disabled) ;;
    *) fail "DARS_PI_PREFLIGHT must be basic, active, or disabled" ;;
  esac

  [ -x "${DARS_PI_PATH}" ] || fail "Pi executable is not executable: ${DARS_PI_PATH}"
  "${DARS_PI_PATH}" --version >/dev/null || fail "Pi version check failed"
  resolve_pi_model

  if [ "$mode" = disabled ]; then
    log "Pi preflight disabled explicitly for ${PI_PROVIDER}/${PI_MODEL}"
    return
  fi

  local scratch catalog auth_json
  scratch="$(mktemp -d)"
  catalog="$scratch/models.txt"
  auth_json="$scratch/auth.json"
  trap 'rm -rf "$scratch"' RETURN

  "${DARS_PI_PATH}" --list-models >"$catalog" 2>/dev/null || fail "Pi model catalog failed"
  pi_catalog_contains_model "$catalog" || fail "Pi model not found: ${PI_PROVIDER}/${PI_MODEL}"

  if [ ! -w "${PI_CODING_AGENT_DIR}" ]; then
    pi_auth_check "$auth_json" --no-refresh
  else
    pi_auth_check "$auth_json"
  fi
  if [ "$PI_AUTH_STATE" = ready ]; then
    log "Pi basic preflight ready for ${PI_PROVIDER}/${PI_MODEL}"
  elif [ "$PI_AUTH_STATE" = custom ]; then
    [ "$mode" = active ] || fail "Pi extension/custom Provider requires DARS_PI_PREFLIGHT=active: ${PI_PROVIDER}/${PI_MODEL}"
    log "Pi extension/custom Provider will be validated by active probe: ${PI_PROVIDER}/${PI_MODEL}"
  else
    fail "Pi auth is not ready; fix the built-in Provider credential or use active for an extension/custom Provider"
  fi

  if [ "$mode" = active ]; then
    local active_dir active_output
    active_dir="$scratch/active"
    active_output="$scratch/active.jsonl"
    mkdir -p "$active_dir"
    (
      cd "$active_dir"
      "${DARS_PI_PATH}" --no-approve --model "${PI_PROVIDER}/${PI_MODEL}" --no-session --no-tools --no-context-files --mode json -p "Reply with exactly DARS_PI_PREFLIGHT_OK" </dev/null >"$active_output"
    ) || fail "Pi active preflight failed"
    grep -Fq "DARS_PI_PREFLIGHT_OK" "$active_output" || fail "Pi active preflight sentinel missing"
    log "Pi active preflight ready for ${PI_PROVIDER}/${PI_MODEL}"
  fi
}

log "validating configuration"
require_value DARS_SERVER_URL
require_value DARS_WORKSPACE_ID
require_value DARS_DAEMON_ID
read_daemon_token

export DARS_PI_PATH="${DARS_PI_PATH:-/usr/local/bin/pi}"
export PI_CODING_AGENT_DIR="${PI_CODING_AGENT_DIR:-/config/pi}"
export DARS_WORKSPACES_ROOT="${DARS_WORKSPACES_ROOT:-/workspaces}"
export HOME="${HOME:-/home/dars}"

[ -d "${PI_CODING_AGENT_DIR}" ] || fail "Pi config directory does not exist: ${PI_CODING_AGENT_DIR}"
[ -r "${PI_CODING_AGENT_DIR}" ] || fail "Pi config directory is not readable by uid $(id -u): ${PI_CODING_AGENT_DIR}"
mkdir -p "${HOME}/.dars" "${DARS_WORKSPACES_ROOT}" || fail "could not create DARS state/workspace directories as uid $(id -u)"
[ -w "${HOME}/.dars" ] || fail "DARS state directory is not writable by uid $(id -u): ${HOME}/.dars"
[ -w "${DARS_WORKSPACES_ROOT}" ] || fail "workspace directory is not writable by uid $(id -u): ${DARS_WORKSPACES_ROOT}"

log "Pi preflight started"
run_pi_preflight

log "DARS daemon-token auth preflight started"
dars daemon auth-preflight >/dev/null || fail "DARS daemon-token auth preflight failed"
log "DARS authenticated"

log "starting daemon"
exec dars daemon start --foreground
