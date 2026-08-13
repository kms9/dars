-- name: CancelAgentTasksByAgent :many
UPDATE agent_task_queue task
SET status = 'cancelled',
    completed_at = COALESCE(task.completed_at, now()),
    prepare_lease_expires_at = NULL,
    chat_finalize_deferred_at = CASE
        WHEN task.chat_session_id IS NOT NULL
             AND task.status IN ('dispatched', 'waiting_local_directory', 'running') THEN now()
        ELSE task.chat_finalize_deferred_at
    END
WHERE task.workspace_id = sqlc.arg(workspace_id)
  AND task.agent_id = sqlc.arg(agent_id)
  AND task.status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running')
RETURNING task.*;

-- name: ListAgentTasksPage :many
SELECT task.*
FROM agent_task_queue task
JOIN agent ON agent.id = task.agent_id
WHERE task.workspace_id = sqlc.arg(workspace_id)
  AND task.agent_id = sqlc.arg(agent_id)
  AND agent.kind = 'user'
  AND (
      sqlc.narg(cursor_created_at)::timestamptz IS NULL
      OR (task.created_at, task.id) < (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
  )
ORDER BY task.created_at DESC, task.id DESC
LIMIT sqlc.arg(page_limit);

-- name: GetAgentTaskSummary30d :one
SELECT
    COUNT(*)::int AS run_count,
    COUNT(*) FILTER (WHERE task.status = 'completed')::int AS success_count,
    COUNT(*) FILTER (WHERE task.status = 'failed')::int AS fail_count,
    COALESCE(
        AVG(EXTRACT(EPOCH FROM (task.completed_at - task.started_at)) * 1000)
            FILTER (WHERE task.started_at IS NOT NULL AND task.completed_at IS NOT NULL),
        0
    )::float8 AS avg_duration_ms
FROM agent_task_queue task
JOIN agent ON agent.id = task.agent_id
WHERE task.workspace_id = sqlc.arg(workspace_id)
  AND task.agent_id = sqlc.arg(agent_id)
  AND agent.kind = 'user'
  AND task.created_at > now() - INTERVAL '30 days'
  AND task.status IN ('completed', 'failed');

-- name: GetWorkspaceAgentRunCounts :many
SELECT
    atq.agent_id,
    COUNT(*)::int AS run_count
FROM agent_task_queue atq
JOIN agent a ON a.id = atq.agent_id
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.kind = 'user'
  AND atq.created_at > now() - INTERVAL '30 days'
GROUP BY atq.agent_id;

-- name: GetWorkspaceAgentActivity30d :many
SELECT
    atq.agent_id,
    DATE_TRUNC('day', atq.completed_at)::timestamptz AS bucket,
    COUNT(*)::int AS task_count,
    COUNT(*) FILTER (WHERE atq.status = 'failed')::int AS failed_count
FROM agent_task_queue atq
JOIN agent a ON a.id = atq.agent_id
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.kind = 'user'
  AND atq.completed_at IS NOT NULL
  AND atq.completed_at > now() - INTERVAL '30 days'
GROUP BY atq.agent_id, bucket
ORDER BY atq.agent_id, bucket;

-- name: ListWorkspaceAgentTaskSnapshot :many
SELECT atq.*
FROM agent_task_queue atq
JOIN agent a ON a.id = atq.agent_id
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.kind = 'user'
  AND atq.status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')

UNION ALL

SELECT latest.*
FROM agent a
JOIN LATERAL (
    SELECT atq.*
    FROM agent_task_queue atq
    WHERE atq.agent_id = a.id
      AND atq.status IN ('completed', 'failed')
    ORDER BY atq.completed_at DESC NULLS LAST, atq.created_at DESC, atq.id DESC
    LIMIT 1
) latest ON TRUE
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.kind = 'user';

-- name: UpdateAgentAvatar :one
UPDATE agent
SET avatar_url = sqlc.narg(avatar_url), updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND kind = 'user'
RETURNING *;

-- name: UpdateSquadAvatar :one
UPDATE squad
SET avatar_url = sqlc.narg(avatar_url), updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: GetAgentByAvatarURL :one
SELECT id, workspace_id FROM agent
WHERE avatar_url = sqlc.arg(avatar_url) AND kind = 'user'
LIMIT 1;

-- name: GetSquadByAvatarURL :one
SELECT id, workspace_id FROM squad
WHERE avatar_url = sqlc.arg(avatar_url)
LIMIT 1;
