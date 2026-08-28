#!/usr/bin/env bash
set -euo pipefail

if [ "${DARS_RUN_REAL_PI_CONTAINER_SMOKE:-0}" != 1 ]; then
  echo "real daemon-pi smoke skipped; set DARS_RUN_REAL_PI_CONTAINER_SMOKE=1" >&2
  exit 0
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env}"
PI_SOURCE="${DARS_REAL_PI_CONFIG_DIR:-/Users/logo/.pi/agent}"
PI_MODEL="${DARS_REAL_PI_MODEL:-xai/grok-4.6}"
IMAGE="${DARS_DAEMON_PI_IMAGE:-dars-daemon-pi:real-smoke}"
TEST_ROOT="$(mktemp -d)"
RUN_SUFFIX="$(date -u +%Y%m%d%H%M%S)-$$"
DB_NAME="dars_lightweight_check_pi_${RUN_SUFFIX//-/_}"
CONTAINER_NAME="dars-daemon-pi-real-${RUN_SUFFIX}"
STATE_VOLUME="dars_daemon_pi_state_${RUN_SUFFIX//-/_}"
WORKSPACE_VOLUME="dars_daemon_pi_workspaces_${RUN_SUFFIX//-/_}"
SERVER_PID=""
DB_CREATED=false
CONTAINER_CREATED=false

log() {
  printf '[real-daemon-pi-smoke] %s\n' "$*" >&2
}

fail() {
  log "FAILED: $*"
  exit 1
}

[ -f "$ENV_FILE" ] || fail "missing env file: $ENV_FILE"
[ -d "$PI_SOURCE" ] || fail "missing Pi config directory: $PI_SOURCE"
command -v docker >/dev/null || fail "docker is required"
command -v jq >/dev/null || fail "jq is required"
command -v psql >/dev/null || fail "psql is required"
command -v curl >/dev/null || fail "curl is required"

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

SOURCE_DATABASE_URL="${DATABASE_URL:-}"
[ -n "$SOURCE_DATABASE_URL" ] || fail "DATABASE_URL is required"
SOURCE_DATABASE_NO_QUERY="${SOURCE_DATABASE_URL%%\?*}"
SOURCE_DATABASE_QUERY=""
if [[ "$SOURCE_DATABASE_URL" == *\?* ]]; then
  SOURCE_DATABASE_QUERY="?${SOURCE_DATABASE_URL#*\?}"
fi
case "$SOURCE_DATABASE_NO_QUERY" in
  postgres://*|postgresql://*) ;;
  *) fail "unsupported DATABASE_URL" ;;
esac
case "$SOURCE_DATABASE_NO_QUERY" in
  *@localhost:*/*|*@localhost/*|*@127.0.0.1:*/*|*@127.0.0.1/*|*@\[::1\]:*/*|*@\[::1\]/*) ;;
  *) fail "refusing non-loopback PostgreSQL: $SOURCE_DATABASE_NO_QUERY" ;;
esac
DATABASE_AUTHORITY="${SOURCE_DATABASE_NO_QUERY%/*}"
ADMIN_DATABASE_URL="${DATABASE_AUTHORITY}/postgres${SOURCE_DATABASE_QUERY}"
CHECK_DATABASE_URL="${DATABASE_AUTHORITY}/${DB_NAME}${SOURCE_DATABASE_QUERY}"

cleanup() {
  local rc=$?
  trap - EXIT
  if [ "$CONTAINER_CREATED" = true ]; then
    docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  fi
  if [ -n "$SERVER_PID" ]; then
    kill -TERM "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
  fi
  docker volume rm "$STATE_VOLUME" "$WORKSPACE_VOLUME" >/dev/null 2>&1 || true
  if [ "$DB_CREATED" = true ]; then
    psql "$ADMIN_DATABASE_URL" -X --set ON_ERROR_STOP=1 --set database_name="$DB_NAME" >/dev/null <<'SQL' || true
SELECT format('DROP DATABASE IF EXISTS %I WITH (FORCE)', :'database_name') \gexec
SQL
  fi
  chmod -R u+w "$TEST_ROOT" >/dev/null 2>&1 || true
  rm -rf "$TEST_ROOT"
  exit "$rc"
}
trap cleanup EXIT

log "creating isolated PostgreSQL database $DB_NAME"
bash "$ROOT_DIR/scripts/ensure-postgres.sh" "$ENV_FILE" >/dev/null
psql "$ADMIN_DATABASE_URL" -X --set ON_ERROR_STOP=1 --set database_name="$DB_NAME" >/dev/null <<'SQL'
SELECT format('DROP DATABASE IF EXISTS %I WITH (FORCE)', :'database_name') \gexec
SELECT format('CREATE DATABASE %I', :'database_name') \gexec
SQL
DB_CREATED=true

export DATABASE_URL="$CHECK_DATABASE_URL"
export LIGHTWEIGHT_DATABASE_URL="$CHECK_DATABASE_URL"
export EXPECTED_DATABASE_NAME="$DB_NAME"
export DARS_EDITION=lightweight
export APP_ENV=development
export ALLOW_SIGNUP=true
export DARS_DEV_VERIFICATION_CODE=424242
export DARS_AGENT_SECRET_KEY="$(openssl rand -base64 32)"
export DARS_AGENT_SECRET_KEY_ID=daemon-pi-smoke
export JWT_SECRET="$(openssl rand -hex 32)"

(cd "$ROOT_DIR/server" && go run ./cmd/migrate up) >/dev/null
(cd "$ROOT_DIR/server" && go build -o "$TEST_ROOT/dars-server" ./cmd/server)

SERVER_PORT=$((19000 + $$ % 1000))
while lsof -ti ":$SERVER_PORT" >/dev/null 2>&1; do
  SERVER_PORT=$((SERVER_PORT + 1))
done
SERVER_URL="http://127.0.0.1:$SERVER_PORT"
CONTAINER_SERVER_URL="http://host.docker.internal:$SERVER_PORT"
SERVER_LOG="$TEST_ROOT/server.log"

start_server() {
  log "starting isolated DARS Server on $SERVER_URL"
  env DATABASE_URL="$CHECK_DATABASE_URL" LIGHTWEIGHT_DATABASE_URL="$CHECK_DATABASE_URL" EXPECTED_DATABASE_NAME="$DB_NAME" DARS_EDITION=lightweight APP_ENV=development ALLOW_SIGNUP=true DARS_DEV_VERIFICATION_CODE=424242 DARS_AGENT_SECRET_KEY="$DARS_AGENT_SECRET_KEY" DARS_AGENT_SECRET_KEY_ID="$DARS_AGENT_SECRET_KEY_ID" JWT_SECRET="$JWT_SECRET" PORT="$SERVER_PORT" BACKEND_PORT="$SERVER_PORT" FRONTEND_ORIGIN="$SERVER_URL" DARS_APP_URL="$SERVER_URL" DARS_PUBLIC_URL="$CONTAINER_SERVER_URL" "$TEST_ROOT/dars-server" >>"$SERVER_LOG" 2>&1 &
  SERVER_PID=$!
  for _ in {1..90}; do
    if curl -fsS "$SERVER_URL/readyz" >/dev/null 2>&1; then
      return
    fi
    if ! kill -0 "$SERVER_PID" >/dev/null 2>&1; then
      fail "DARS Server exited before readiness"
    fi
    sleep 1
  done
  fail "DARS Server readiness timed out"
}

stop_server() {
  if [ -n "$SERVER_PID" ]; then
    kill -TERM "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
    SERVER_PID=""
  fi
}

start_server

request_status() {
  local method="$1" path="$2" body="$3" output="$4"
  shift 4
  local -a args=(-sS -o "$output" -w '%{http_code}' -X "$method" "$SERVER_URL$path")
  if [ -n "$body" ]; then
    args+=(-H 'Content-Type: application/json' --data-binary "@$body")
  fi
  args+=("$@")
  curl "${args[@]}"
}

expect_request() {
  local expected="$1" method="$2" path="$3" body="$4" output="$5"
  shift 5
  local actual
  actual="$(request_status "$method" "$path" "$body" "$output" "$@")"
  [ "$actual" = "$expected" ] || fail "$method $path returned HTTP $actual, want $expected"
}

EMAIL="daemon-pi-smoke-${RUN_SUFFIX}@example.test"
jq -n --arg email "$EMAIL" '{email:$email}' >"$TEST_ROOT/send-code.json"
expect_request 202 POST /auth/send-code "$TEST_ROOT/send-code.json" "$TEST_ROOT/send-code-response.json"
jq -n --arg email "$EMAIL" '{email:$email,code:"424242"}' >"$TEST_ROOT/verify-code.json"
expect_request 200 POST /auth/verify-code "$TEST_ROOT/verify-code.json" "$TEST_ROOT/login.json"
HUMAN_TOKEN="$(jq -er .token "$TEST_ROOT/login.json")"

WORKSPACE_SLUG="pi-smoke-$(printf '%s' "$RUN_SUFFIX" | tr '[:upper:]' '[:lower:]')"
jq -n --arg name "daemon-pi real smoke" --arg slug "$WORKSPACE_SLUG" '{name:$name,slug:$slug}' >"$TEST_ROOT/workspace.json"
expect_request 201 POST /api/workspaces "$TEST_ROOT/workspace.json" "$TEST_ROOT/workspace-response.json" -H "Authorization: Bearer $HUMAN_TOKEN"
WORKSPACE_ID="$(jq -er .id "$TEST_ROOT/workspace-response.json")"
[ "$(jq -c .repos "$TEST_ROOT/workspace-response.json")" = '[]' ] || fail "smoke Workspace repos is not empty"

DAEMON_ID="daemon-pi-real-${RUN_SUFFIX}"
jq -n --arg daemon_id "$DAEMON_ID" '{daemon_id:$daemon_id,name:"daemon-pi real smoke"}' >"$TEST_ROOT/daemon-token-request.json"
expect_request 201 POST "/api/workspaces/$WORKSPACE_ID/daemon-tokens" "$TEST_ROOT/daemon-token-request.json" "$TEST_ROOT/daemon-token-response.json" -H "Authorization: Bearer $HUMAN_TOKEN" -H "X-Workspace-ID: $WORKSPACE_ID"
DAEMON_TOKEN="$(jq -er .token "$TEST_ROOT/daemon-token-response.json")"
DAEMON_TOKEN_ID="$(jq -er .id "$TEST_ROOT/daemon-token-response.json")"
printf '%s\n' "$DAEMON_TOKEN" >"$TEST_ROOT/daemon-token.secret"
chmod 600 "$TEST_ROOT/daemon-token.secret"
DAEMON_TOKEN=""
: >"$TEST_ROOT/daemon-token-response.json"

log "copying Pi config into a mode-0700 temporary mount"
mkdir -p "$TEST_ROOT/pi-config"
chmod 700 "$TEST_ROOT/pi-config"
cp -R "$PI_SOURCE/." "$TEST_ROOT/pi-config/"
chmod -R go-rwx "$TEST_ROOT/pi-config"

if [ "${DARS_DAEMON_PI_SKIP_BUILD:-0}" != 1 ]; then
  build_uid="$(id -u)"
  build_gid="$(id -g)"
  if [ "$build_uid" -eq 0 ]; then build_uid=1000; fi
  if [ "$build_gid" -eq 0 ]; then build_gid=1000; fi
  log "building daemon-pi image"
  docker build --progress=plain --build-arg "DARS_UID=$build_uid" --build-arg "DARS_GID=$build_gid" -f "$ROOT_DIR/Dockerfile.daemon-pi" -t "$IMAGE" "$ROOT_DIR" >/dev/null
fi
docker volume create "$STATE_VOLUME" >/dev/null
docker volume create "$WORKSPACE_VOLUME" >/dev/null

container_args=(
  --name "$CONTAINER_NAME"
  --add-host host.docker.internal:host-gateway
  --env "DARS_SERVER_URL=$CONTAINER_SERVER_URL"
  --env "DARS_WORKSPACE_ID=$WORKSPACE_ID"
  --env "DARS_DAEMON_ID=$DAEMON_ID"
  --env "DARS_DAEMON_DEVICE_NAME=daemon-pi-real-smoke"
  --env "DARS_AGENT_RUNTIME_NAME=DARS Pi Real Smoke"
  --env "DARS_DAEMON_MAX_CONCURRENT_TASKS=1"
  --env "DARS_DAEMON_TOKEN_FILE=/run/secrets/dars_daemon_token"
  --env "DARS_PI_MODEL=$PI_MODEL"
  --env "DARS_PI_PREFLIGHT=active"
  --mount "type=bind,src=$TEST_ROOT/daemon-token.secret,dst=/run/secrets/dars_daemon_token,readonly"
  --mount "type=bind,src=$TEST_ROOT/pi-config,dst=/config/pi"
  --mount "type=volume,src=$STATE_VOLUME,dst=/home/dars/.dars"
  --mount "type=volume,src=$WORKSPACE_VOLUME,dst=/workspaces"
)

start_container() {
  log "starting daemon-pi with active Pi preflight"
  docker run --detach "${container_args[@]}" "$IMAGE" >/dev/null
  CONTAINER_CREATED=true
  for _ in {1..300}; do
    local state health
    state="$(docker inspect "$CONTAINER_NAME" --format '{{.State.Status}}')"
    health="$(docker inspect "$CONTAINER_NAME" --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}')"
    if [ "$health" = healthy ]; then
      [ "$(docker inspect "$CONTAINER_NAME" --format '{{.Path}}')" = /usr/bin/tini ] || fail "daemon-pi PID 1 is not tini"
      [ "$(docker exec "$CONTAINER_NAME" id -u)" != 0 ] || fail "daemon-pi is running as root"
      return
    fi
    if [ "$state" != running ]; then
      docker logs "$CONTAINER_NAME" 2>&1 | grep '^\[daemon-pi\]' >&2 || true
      fail "daemon-pi exited during startup"
    fi
    sleep 1
  done
  fail "daemon-pi did not become healthy"
}

stop_container() {
  docker stop --time 20 "$CONTAINER_NAME" >/dev/null
  docker rm "$CONTAINER_NAME" >/dev/null
  CONTAINER_CREATED=false
}

db_query() {
  psql "$CHECK_DATABASE_URL" -XAtq --set ON_ERROR_STOP=1 -c "$1"
}

wait_for_runtime() {
  for _ in {1..90}; do
    RUNTIME_ID="$(db_query "SELECT id::text FROM agent_runtime WHERE workspace_id = '$WORKSPACE_ID'::uuid AND daemon_id = '$DAEMON_ID' AND lower(provider) = 'pi' AND status = 'online' ORDER BY id LIMIT 1")"
    if [ -n "$RUNTIME_ID" ]; then
      return
    fi
    sleep 1
  done
  fail "Pi Runtime did not register online"
}

start_container
wait_for_runtime
INITIAL_RUNTIME_ID="$RUNTIME_ID"

AGENT_INSTRUCTIONS='For each task, use the bash tool exactly once to run `printf DARS_PI_E2E_OK`. After observing the tool result, return exactly DARS_PI_E2E_OK with no markdown, explanation, punctuation, or surrounding whitespace.'
jq -n --arg name "Pi E2E Agent" --arg runtime_id "$RUNTIME_ID" --arg model "$PI_MODEL" --arg instructions "$AGENT_INSTRUCTIONS" '{name:$name,runtime_id:$runtime_id,model:$model,instructions:$instructions,permission_mode:"private",max_concurrent_tasks:1}' >"$TEST_ROOT/agent.json"
expect_request 201 POST /api/agents "$TEST_ROOT/agent.json" "$TEST_ROOT/agent-response.json" -H "Authorization: Bearer $HUMAN_TOKEN" -H "X-Workspace-ID: $WORKSPACE_ID"
AGENT_ID="$(jq -er .id "$TEST_ROOT/agent-response.json")"
[ "$(db_query "SELECT count(*) FROM agent_tool_bundle_head WHERE workspace_id = '$WORKSPACE_ID'::uuid AND agent_id = '$AGENT_ID'::uuid")" = 0 ] || fail "Agent unexpectedly has a ToolBundle"

dispatch_issue() {
  local agent_id="$1" label="$2"
  jq -n --arg title "$label" --arg description 'Execute the acceptance command through the bash tool and finish with the exact required sentinel.' --arg agent_id "$agent_id" '{title:$title,description:$description,status:"todo",assignee_type:"agent",assignee_id:$agent_id,acceptance_criteria:["tool result contains DARS_PI_E2E_OK","final output is exactly DARS_PI_E2E_OK"],context_refs:[]}' >"$TEST_ROOT/issue-request.json"
  expect_request 201 POST /api/issues "$TEST_ROOT/issue-request.json" "$TEST_ROOT/issue-response.json" -H "Authorization: Bearer $HUMAN_TOKEN" -H "X-Workspace-ID: $WORKSPACE_ID" -H "Idempotency-Key: $RUN_SUFFIX-$label"
  ISSUE_ID="$(jq -er .id "$TEST_ROOT/issue-response.json")"
  for _ in {1..30}; do
    TASK_ID="$(db_query "SELECT id::text FROM agent_task_queue WHERE workspace_id = '$WORKSPACE_ID'::uuid AND issue_id = '$ISSUE_ID'::uuid ORDER BY created_at LIMIT 1")"
    if [ -n "$TASK_ID" ]; then
      return
    fi
    sleep 1
  done
  fail "task was not created for issue $ISSUE_ID"
}

wait_for_task() {
  local task_id="$1" wanted="$2"
  for _ in {1..360}; do
    local current
    current="$(db_query "SELECT status FROM agent_task_queue WHERE id = '$task_id'::uuid")"
    if [ "$current" = "$wanted" ]; then
      return
    fi
    if [[ "$current" =~ ^(completed|failed|cancelled)$ ]] && [ "$current" != "$wanted" ]; then
      fail "task $task_id reached $current, want $wanted"
    fi
    sleep 1
  done
  fail "task $task_id did not reach $wanted"
}

assert_success_task() {
  local task_id="$1" issue_id="$2"
  [ "$(db_query "SELECT trim(result->>'output') FROM agent_task_queue WHERE id = '$task_id'::uuid")" = DARS_PI_E2E_OK ] || fail "task result output is not exact"
  [ "$(db_query "SELECT count(*) FROM task_message WHERE task_id = '$task_id'::uuid AND type = 'tool_result' AND output LIKE '%DARS_PI_E2E_OK%'")" -ge 1 ] || fail "task has no sentinel tool-result"
  [ "$(db_query "SELECT count(*) FROM task_message WHERE task_id = '$task_id'::uuid AND type = 'tool_use'")" -ge 1 ] || fail "task has no tool-use"
  [ "$(db_query "SELECT count(*) FROM agent_task_queue WHERE issue_id = '$issue_id'::uuid AND status IN ('completed','failed','cancelled')")" = 1 ] || fail "issue has a non-unique terminal task"
  [ "$(db_query "SELECT count(*) FROM comment WHERE source_task_id = '$task_id'::uuid AND content = 'DARS_PI_E2E_OK'")" = 1 ] || fail "completion projection is not exact"
  [ "$(db_query "SELECT count(*) FROM task_usage WHERE task_id = '$task_id'::uuid")" -ge 1 ] || fail "Pi usage was not persisted"
  [ "$(db_query "SELECT CASE WHEN tool_bundle_id IS NULL THEN 1 ELSE 0 END FROM agent_task_queue WHERE id = '$task_id'::uuid")" = 1 ] || fail "task unexpectedly has a ToolBundle"
}

log "dispatching first real Pi task"
dispatch_issue "$AGENT_ID" first-success
FIRST_ISSUE_ID="$ISSUE_ID"
FIRST_TASK_ID="$TASK_ID"
wait_for_task "$FIRST_TASK_ID" completed
assert_success_task "$FIRST_TASK_ID" "$FIRST_ISSUE_ID"

log "recreating daemon-pi with the same token and volumes"
stop_container
start_container
wait_for_runtime
[ "$RUNTIME_ID" = "$INITIAL_RUNTIME_ID" ] || fail "Runtime identity changed after container recreation"
[ "$(db_query "SELECT count(*) FROM agent_runtime WHERE workspace_id = '$WORKSPACE_ID'::uuid AND daemon_id = '$DAEMON_ID' AND lower(provider) = 'pi'")" = 1 ] || fail "container recreation duplicated Pi Runtime"

dispatch_issue "$AGENT_ID" second-success
SECOND_ISSUE_ID="$ISSUE_ID"
SECOND_TASK_ID="$TASK_ID"
wait_for_task "$SECOND_TASK_ID" completed
assert_success_task "$SECOND_TASK_ID" "$SECOND_ISSUE_ID"

log "verifying Server disconnect and daemon reconnect"
HEARTBEAT_BEFORE="$(db_query "SELECT extract(epoch FROM last_seen_at)::bigint FROM agent_runtime WHERE id = '$RUNTIME_ID'::uuid")"
stop_server
sleep 5
start_server
for _ in {1..90}; do
  HEARTBEAT_AFTER="$(db_query "SELECT extract(epoch FROM last_seen_at)::bigint FROM agent_runtime WHERE id = '$RUNTIME_ID'::uuid")"
  if [ "$HEARTBEAT_AFTER" -gt "$HEARTBEAT_BEFORE" ]; then
    break
  fi
  sleep 1
done
[ "${HEARTBEAT_AFTER:-0}" -gt "$HEARTBEAT_BEFORE" ] || fail "daemon did not heartbeat after Server restart"
[ "$(db_query "SELECT count(*) FROM agent_runtime WHERE workspace_id = '$WORKSPACE_ID'::uuid AND daemon_id = '$DAEMON_ID' AND lower(provider) = 'pi'")" = 1 ] || fail "Server reconnect duplicated Pi Runtime"

jq -n --arg name "Pi failure Agent" --arg runtime_id "$RUNTIME_ID" --arg model "$PI_MODEL" '{name:$name,runtime_id:$runtime_id,model:$model,instructions:"This task is expected to fail before model execution.",custom_args:["--dars-intentionally-invalid-flag"],permission_mode:"private",max_concurrent_tasks:1}' >"$TEST_ROOT/bad-agent.json"
expect_request 201 POST /api/agents "$TEST_ROOT/bad-agent.json" "$TEST_ROOT/bad-agent-response.json" -H "Authorization: Bearer $HUMAN_TOKEN" -H "X-Workspace-ID: $WORKSPACE_ID"
BAD_AGENT_ID="$(jq -er .id "$TEST_ROOT/bad-agent-response.json")"

log "verifying one Pi task failure does not crash the daemon"
docker pause "$CONTAINER_NAME" >/dev/null
dispatch_issue "$BAD_AGENT_ID" expected-failure
FAILED_ISSUE_ID="$ISSUE_ID"
FAILED_TASK_ID="$TASK_ID"
db_query "UPDATE agent_task_queue SET max_attempts = 1 WHERE id = '$FAILED_TASK_ID'::uuid" >/dev/null
docker unpause "$CONTAINER_NAME" >/dev/null
wait_for_task "$FAILED_TASK_ID" failed
[ "$(docker inspect "$CONTAINER_NAME" --format '{{.State.Running}}')" = true ] || fail "daemon-pi stopped after a failed Pi task"

dispatch_issue "$AGENT_ID" recovery-success
RECOVERY_ISSUE_ID="$ISSUE_ID"
RECOVERY_TASK_ID="$TASK_ID"
wait_for_task "$RECOVERY_TASK_ID" completed
assert_success_task "$RECOVERY_TASK_ID" "$RECOVERY_ISSUE_ID"

IMAGE_ID="$(docker image inspect "$IMAGE" --format '{{.Id}}')"
VERSIONS="$(docker run --rm --entrypoint bash "$IMAGE" -c 'printf "%s|%s|" "$(node --version)" "$(pi --version)"; dars version 2>/dev/null | head -1')"
NODE_VERSION="${VERSIONS%%|*}"
VERSION_REST="${VERSIONS#*|}"
PI_VERSION="${VERSION_REST%%|*}"
DARS_VERSION="${VERSION_REST#*|}"
FINISHED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

jq -n --arg finished_at "$FINISHED_AT" --arg image "$IMAGE" --arg image_id "$IMAGE_ID" --arg dars_version "$DARS_VERSION" --arg node_version "$NODE_VERSION" --arg pi_version "$PI_VERSION" --arg pi_model "$PI_MODEL" --arg workspace_id "$WORKSPACE_ID" --arg daemon_token_id "$DAEMON_TOKEN_ID" --arg daemon_id "$DAEMON_ID" --arg runtime_id "$RUNTIME_ID" --arg first_task_id "$FIRST_TASK_ID" --arg second_task_id "$SECOND_TASK_ID" --arg failed_task_id "$FAILED_TASK_ID" --arg recovery_task_id "$RECOVERY_TASK_ID" '{schema_version:1,status:"passed",finished_at:$finished_at,image:{name:$image,id:$image_id,dars:$dars_version,node:$node_version,pi:$pi_version,non_root:true,pid_1:"tini"},pi_model:$pi_model,preflight:{mode:"active",status:"passed"},workspace:{id:$workspace_id,repos_empty:true},daemon:{id:$daemon_id,token_record_id:$daemon_token_id,runtime_id:$runtime_id,registered_online:true,heartbeat:true,reused_identity:true,reconnected:true},tasks:{first:{id:$first_task_id,status:"completed",tool_result:true,usage_persisted:true,unique_terminal:true,completion_projection:true,exact_output:"DARS_PI_E2E_OK",tool_bundle:null},after_recreate:{id:$second_task_id,status:"completed",exact_output:"DARS_PI_E2E_OK"},expected_failure:{id:$failed_task_id,status:"failed",daemon_survived:true},recovery:{id:$recovery_task_id,status:"completed",exact_output:"DARS_PI_E2E_OK"}},secrets_retained:false}'
