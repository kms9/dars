-- name: CreateAgentBuilderCarrier :one
INSERT INTO agent (
    workspace_id, runtime_id, owner_id, name, description, instructions,
    runtime_config, max_concurrent_tasks, custom_env, custom_args, mcp_config,
    model, permission_mode, disabled_runtime_skills, kind, system_key
)
VALUES (
    sqlc.arg(workspace_id), sqlc.arg(runtime_id), sqlc.arg(owner_id), sqlc.arg(name),
    '', sqlc.arg(instructions), '{}'::jsonb, 1, '{}'::jsonb, '[]'::jsonb, '{}'::jsonb,
    sqlc.narg(model), 'private', '[]'::jsonb, 'system', sqlc.arg(system_key)
)
RETURNING *;

-- name: GetAgentBuilderCarrier :one
SELECT * FROM agent
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND kind = 'system'
  AND system_key LIKE 'agent_builder:%';

-- name: LockAgentBuilderCarrier :one
SELECT * FROM agent
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND kind = 'system'
  AND system_key LIKE 'agent_builder:%'
FOR UPDATE;

-- name: UpdateAgentBuilderCarrierRuntime :one
UPDATE agent
SET runtime_id = sqlc.arg(runtime_id),
    model = sqlc.narg(model),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND kind = 'system'
  AND system_key LIKE 'agent_builder:%'
RETURNING *;

-- name: DeleteAgentBuilderCarrier :exec
DELETE FROM agent
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND kind = 'system'
  AND system_key LIKE 'agent_builder:%';

-- name: UpsertAgentBuilderDraft :one
INSERT INTO agent_builder_draft (chat_session_id, workspace_id, draft)
VALUES (sqlc.arg(chat_session_id), sqlc.arg(workspace_id), sqlc.arg(draft))
ON CONFLICT (chat_session_id) DO UPDATE
SET draft = EXCLUDED.draft,
    updated_at = now()
RETURNING *;

-- name: GetAgentBuilderDraft :one
SELECT * FROM agent_builder_draft
WHERE chat_session_id = sqlc.arg(chat_session_id)
  AND workspace_id = sqlc.arg(workspace_id);

-- name: DeleteAgentBuilderDraft :exec
DELETE FROM agent_builder_draft WHERE chat_session_id = sqlc.arg(chat_session_id);

-- name: DeleteAgentBuilderDraftsByWorkspace :execrows
DELETE FROM agent_builder_draft WHERE workspace_id = sqlc.arg(workspace_id);

-- name: ListAgentAvatarURLsByWorkspace :many
SELECT avatar_url FROM agent
WHERE workspace_id = sqlc.arg(workspace_id)
  AND avatar_url IS NOT NULL
  AND avatar_url <> '';

-- name: ListSquadAvatarURLsByWorkspace :many
SELECT avatar_url FROM squad
WHERE workspace_id = sqlc.arg(workspace_id)
  AND avatar_url IS NOT NULL
  AND avatar_url <> '';
