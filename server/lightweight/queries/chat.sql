-- name: CreateChatSession :one
INSERT INTO chat_session (
    workspace_id, agent_id, creator_id, runtime_id, title, status
)
VALUES (
    sqlc.arg(workspace_id), sqlc.arg(agent_id), sqlc.arg(creator_id),
    sqlc.arg(runtime_id), sqlc.arg(title), 'active'
)
RETURNING *;

-- name: GetChatSession :one
SELECT * FROM chat_session
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: LockChatSession :one
SELECT * FROM chat_session
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
FOR UPDATE;

-- name: ListAgentBuilderSessionsByCreator :many
SELECT cs.id,
       cs.title,
       cs.created_at,
       cs.updated_at,
       a.runtime_id,
       COALESCE(lm.content, '') AS last_message_content,
       COALESCE(lm.role, '') AS last_message_role,
       lm.created_at AS last_message_at,
       d.draft AS stored_draft
FROM chat_session cs
JOIN agent a ON a.id = cs.agent_id
LEFT JOIN agent_builder_draft d ON d.chat_session_id = cs.id
LEFT JOIN LATERAL (
  SELECT content, role, created_at
    FROM chat_message m
   WHERE m.chat_session_id = cs.id
   ORDER BY m.created_at DESC
   LIMIT 1
) lm ON true
WHERE cs.workspace_id = sqlc.arg(workspace_id)
  AND cs.creator_id = sqlc.arg(creator_id)
  AND cs.status = 'active'
  AND a.kind = 'system'
  AND a.system_key LIKE 'agent_builder:%'
  AND (lm.created_at IS NOT NULL OR d.chat_session_id IS NOT NULL)
ORDER BY COALESCE(lm.created_at, d.updated_at, cs.updated_at) DESC;

-- name: ListChatSessionsPage :many
SELECT * FROM chat_session
WHERE workspace_id = sqlc.arg(workspace_id)
  AND creator_id = sqlc.arg(creator_id)
  AND (
      sqlc.narg(cursor_updated_at)::timestamptz IS NULL
      OR (updated_at, id) < (sqlc.narg(cursor_updated_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
  )
ORDER BY updated_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: UpdateChatSession :one
UPDATE chat_session
SET title = CASE WHEN sqlc.arg(set_title)::boolean THEN sqlc.arg(title) ELSE title END,
    status = CASE WHEN sqlc.arg(set_status)::boolean THEN sqlc.arg(status) ELSE status END,
    updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: UpdateChatSessionResume :one
UPDATE chat_session
SET session_id = sqlc.narg(session_id), work_dir = sqlc.narg(work_dir), updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: RebindChatSessionRuntime :one
UPDATE chat_session
SET runtime_id = sqlc.arg(runtime_id),
    session_id = CASE WHEN runtime_id = sqlc.arg(runtime_id) THEN session_id ELSE NULL END,
    work_dir = CASE WHEN runtime_id = sqlc.arg(runtime_id) THEN work_dir ELSE NULL END,
    updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: TouchChatSession :one
UPDATE chat_session
SET updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: CountActiveTasksByChatSession :one
SELECT count(*) FROM agent_task_queue
WHERE workspace_id = sqlc.arg(workspace_id)
  AND chat_session_id = sqlc.arg(chat_session_id)
  AND status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running');

-- name: DeleteTaskTokensByChatSession :execrows
DELETE FROM task_token WHERE task_id IN (
    SELECT task.id FROM agent_task_queue task
    WHERE task.workspace_id = sqlc.arg(target_workspace_id)
      AND task.chat_session_id = sqlc.arg(target_chat_session_id)
      AND task.status IN ('completed', 'failed', 'cancelled')
);

-- name: DeleteTaskMessagesByChatSession :execrows
DELETE FROM task_message WHERE task_id IN (
    SELECT task.id FROM agent_task_queue task
    WHERE task.workspace_id = sqlc.arg(target_workspace_id)
      AND task.chat_session_id = sqlc.arg(target_chat_session_id)
      AND task.status IN ('completed', 'failed', 'cancelled')
);

-- name: DeleteTaskUsageByChatSession :execrows
DELETE FROM task_usage WHERE task_id IN (
    SELECT task.id FROM agent_task_queue task
    WHERE task.workspace_id = sqlc.arg(target_workspace_id)
      AND task.chat_session_id = sqlc.arg(target_chat_session_id)
      AND task.status IN ('completed', 'failed', 'cancelled')
);

-- name: DeleteTerminalTasksByChatSession :execrows
DELETE FROM agent_task_queue
WHERE workspace_id = sqlc.arg(workspace_id)
  AND chat_session_id = sqlc.arg(chat_session_id)
  AND status IN ('completed', 'failed', 'cancelled');

-- name: DeleteChatMessagesBySession :execrows
DELETE FROM chat_message WHERE chat_session_id = sqlc.arg(chat_session_id);

-- name: DeleteChatDraftRestoresBySession :execrows
DELETE FROM chat_draft_restore WHERE chat_session_id = sqlc.arg(chat_session_id);

-- name: DeleteChatSession :execrows
DELETE FROM chat_session
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND status = 'archived';

-- name: CreateChatMessage :one
INSERT INTO chat_message (
    chat_session_id, role, content, task_id, failure_reason, elapsed_ms,
    message_kind, idempotency_key, request_hash
)
VALUES (
    sqlc.arg(chat_session_id), sqlc.arg(role), sqlc.arg(content), sqlc.narg(task_id),
    sqlc.narg(failure_reason), sqlc.narg(elapsed_ms), sqlc.arg(message_kind),
    sqlc.narg(idempotency_key), sqlc.narg(request_hash)
)
RETURNING *;

-- name: CreateChatTask :one
WITH identity AS (
    SELECT gen_random_uuid() AS id
)
INSERT INTO agent_task_queue (
    id, workspace_id, agent_id, runtime_id, chat_session_id, status, priority,
    session_id, work_dir, context, initiator_user_id, originator_user_id,
    accountable_user_id, originator_source, chat_input_task_id,
    trigger_evidence_kind, trigger_evidence_ref_id, tool_bundle_id
)
SELECT
    identity.id, sqlc.arg(workspace_id), sqlc.arg(agent_id), sqlc.arg(runtime_id),
    sqlc.arg(chat_session_id), 'queued', 2, sqlc.narg(session_id), sqlc.narg(work_dir),
    '{}'::jsonb, sqlc.arg(user_id), sqlc.arg(user_id), sqlc.arg(user_id),
    'direct_human', identity.id, 'chat', sqlc.arg(chat_session_id),
    (SELECT head.bundle_id FROM agent_tool_bundle_head head
     WHERE head.workspace_id = sqlc.arg(workspace_id) AND head.agent_id = sqlc.arg(agent_id))
FROM identity
RETURNING *;

-- name: GetChatMessageByIdempotencyKey :one
SELECT * FROM chat_message
WHERE chat_session_id = sqlc.arg(chat_session_id)
  AND idempotency_key = sqlc.arg(idempotency_key);

-- name: GetChatMessageByTaskID :one
SELECT * FROM chat_message
WHERE chat_session_id = sqlc.arg(chat_session_id)
  AND task_id = sqlc.arg(task_id)
  AND role = sqlc.arg(role)
ORDER BY created_at, id
LIMIT 1;

-- name: ListChatMessagesPage :many
SELECT * FROM chat_message
WHERE chat_session_id = sqlc.arg(chat_session_id)
  AND (
      sqlc.narg(cursor_created_at)::timestamptz IS NULL
      OR (created_at, id) > (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
  )
ORDER BY created_at, id
LIMIT sqlc.arg(page_limit);

-- name: CreateChatDraftRestore :one
INSERT INTO chat_draft_restore (chat_session_id, task_id, content)
VALUES (sqlc.arg(chat_session_id), sqlc.arg(task_id), sqlc.arg(content))
ON CONFLICT (chat_session_id, task_id) DO UPDATE SET content = EXCLUDED.content
RETURNING *;

-- name: ListChatDraftRestores :many
SELECT * FROM chat_draft_restore
WHERE chat_session_id = sqlc.arg(chat_session_id)
ORDER BY created_at, id;

-- name: DeleteChatDraftRestore :execrows
DELETE FROM chat_draft_restore
WHERE id = sqlc.arg(id) AND chat_session_id = sqlc.arg(chat_session_id);

-- name: GetPendingChatTask :one
SELECT * FROM agent_task_queue
WHERE workspace_id = sqlc.arg(workspace_id)
  AND chat_session_id = sqlc.arg(chat_session_id)
  AND status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running')
ORDER BY created_at DESC, id DESC
LIMIT 1;
