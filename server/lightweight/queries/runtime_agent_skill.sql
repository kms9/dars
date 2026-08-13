-- name: CreateRuntimeProfile :one
INSERT INTO runtime_profile (
    workspace_id, display_name, protocol_family, command_name,
    description, fixed_args, created_by, enabled
)
VALUES (
    sqlc.arg(workspace_id), sqlc.arg(display_name), sqlc.arg(protocol_family),
    sqlc.arg(command_name), sqlc.narg(description), sqlc.arg(fixed_args),
    sqlc.arg(created_by), sqlc.arg(enabled)
)
RETURNING *;

-- name: GetRuntimeProfile :one
SELECT * FROM runtime_profile
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: ListRuntimeProfiles :many
SELECT * FROM runtime_profile
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY enabled DESC, display_name, id;

-- name: UpdateRuntimeProfile :one
UPDATE runtime_profile
SET display_name = CASE WHEN sqlc.arg(set_display_name)::boolean THEN sqlc.arg(display_name) ELSE display_name END,
    protocol_family = CASE WHEN sqlc.arg(set_protocol_family)::boolean THEN sqlc.arg(protocol_family) ELSE protocol_family END,
    command_name = CASE WHEN sqlc.arg(set_command_name)::boolean THEN sqlc.arg(command_name) ELSE command_name END,
    description = CASE WHEN sqlc.arg(set_description)::boolean THEN sqlc.narg(description)::text ELSE description END,
    fixed_args = CASE WHEN sqlc.arg(set_fixed_args)::boolean THEN sqlc.arg(fixed_args)::jsonb ELSE fixed_args END,
    enabled = CASE WHEN sqlc.arg(set_enabled)::boolean THEN sqlc.arg(enabled) ELSE enabled END,
    updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: DeleteRuntimeProfile :execrows
DELETE FROM runtime_profile
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: CountAgentRuntimesByProfile :one
SELECT count(*) FROM agent_runtime
WHERE workspace_id = sqlc.arg(workspace_id) AND profile_id = sqlc.arg(profile_id);

-- name: UpsertAgentRuntime :one
INSERT INTO agent_runtime (
    workspace_id, daemon_id, name, provider, status, device_info,
    metadata, last_seen_at, owner_id, profile_id
)
VALUES (
    sqlc.arg(workspace_id), sqlc.arg(daemon_id), sqlc.arg(name), sqlc.arg(provider),
    sqlc.arg(status), sqlc.arg(device_info), sqlc.arg(metadata), now(),
    sqlc.arg(owner_id), sqlc.narg(profile_id)
)
ON CONFLICT (workspace_id, daemon_id, provider, profile_id)
DO UPDATE SET name = EXCLUDED.name,
              status = EXCLUDED.status,
              device_info = EXCLUDED.device_info,
              metadata = EXCLUDED.metadata,
              last_seen_at = now(),
              owner_id = EXCLUDED.owner_id,
              updated_at = now()
RETURNING *;

-- name: GetAgentRuntime :one
SELECT * FROM agent_runtime
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: ListAgentRuntimes :many
SELECT * FROM agent_runtime
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY status DESC, updated_at DESC, id DESC;

-- name: ListAgentRuntimesByDaemon :many
SELECT * FROM agent_runtime
WHERE workspace_id = sqlc.arg(workspace_id) AND daemon_id = sqlc.arg(daemon_id)
ORDER BY provider, id;

-- name: UpdateAgentRuntimeCustomName :one
UPDATE agent_runtime
SET custom_name = sqlc.narg(custom_name), updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: TouchAgentRuntime :execrows
UPDATE agent_runtime
SET status = 'online', last_seen_at = now(), updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND daemon_id = sqlc.arg(daemon_id);

-- name: TouchAgentRuntimesBatch :execrows
UPDATE agent_runtime
SET status = 'online', last_seen_at = now(), updated_at = now()
WHERE id = ANY(sqlc.arg(runtime_ids)::uuid[]);

-- name: MarkAgentRuntimesOffline :many
UPDATE agent_runtime
SET status = 'offline', updated_at = now()
WHERE workspace_id = sqlc.arg(workspace_id)
  AND daemon_id = sqlc.arg(daemon_id)
RETURNING *;

-- name: ListStaleAgentRuntimes :many
SELECT * FROM agent_runtime
WHERE status = 'online' AND (last_seen_at IS NULL OR last_seen_at < sqlc.arg(stale_before))
ORDER BY last_seen_at NULLS FIRST, id
LIMIT sqlc.arg(batch_limit);

-- name: MarkAgentRuntimeOffline :one
UPDATE agent_runtime
SET status = 'offline', updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND status = 'online'
  AND (last_seen_at IS NULL OR last_seen_at < sqlc.arg(stale_before))
RETURNING *;

-- name: DeleteAgentRuntime :execrows
DELETE FROM agent_runtime
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: LockAgentRuntime :one
SELECT * FROM agent_runtime
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
FOR UPDATE;

-- name: CountActiveTasksByRuntime :one
SELECT count(*) FROM agent_task_queue
WHERE workspace_id = sqlc.arg(workspace_id) AND runtime_id = sqlc.arg(runtime_id)
  AND status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running');

-- name: ClearAgentRuntimeBindings :execrows
UPDATE agent SET runtime_id = NULL, updated_at = now()
WHERE workspace_id = sqlc.arg(workspace_id) AND runtime_id = sqlc.arg(runtime_id);

-- name: CreateAgent :one
INSERT INTO agent (
    workspace_id, runtime_id, owner_id, name, description, instructions,
    runtime_config, max_concurrent_tasks, custom_env, custom_args, mcp_config,
    model, thinking_level, service_tier, permission_mode, disabled_runtime_skills,
    kind
)
VALUES (
    sqlc.arg(workspace_id), sqlc.narg(runtime_id), sqlc.arg(owner_id), sqlc.arg(name),
    sqlc.arg(description), sqlc.arg(instructions), sqlc.arg(runtime_config),
    sqlc.arg(max_concurrent_tasks), sqlc.arg(custom_env), sqlc.arg(custom_args),
    sqlc.arg(mcp_config), sqlc.narg(model), sqlc.narg(thinking_level),
    sqlc.narg(service_tier), sqlc.arg(permission_mode), sqlc.arg(disabled_runtime_skills),
    'user'
)
RETURNING *;

-- name: GetAgentRow :one
SELECT * FROM agent
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: GetAgent :one
SELECT * FROM agent
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND kind = 'user';

-- name: LockAgent :one
SELECT * FROM agent
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND kind = 'user'
FOR UPDATE;

-- name: ListAgents :many
SELECT * FROM agent
WHERE workspace_id = sqlc.arg(workspace_id) AND kind = 'user'
ORDER BY archived_at NULLS FIRST, updated_at DESC, id DESC;

-- name: LockAgentsByRuntime :many
SELECT * FROM agent
WHERE workspace_id = sqlc.arg(workspace_id) AND runtime_id = sqlc.arg(runtime_id)
ORDER BY id
FOR UPDATE;

-- name: CountActiveTasksByAgent :one
SELECT count(*) FROM agent_task_queue
WHERE workspace_id = sqlc.arg(workspace_id) AND agent_id = sqlc.arg(agent_id)
  AND status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running');

-- name: UpdateAgent :one
UPDATE agent
SET name = CASE WHEN sqlc.arg(set_name)::boolean THEN sqlc.arg(name) ELSE name END,
    description = CASE WHEN sqlc.arg(set_description)::boolean THEN sqlc.arg(description) ELSE description END,
    instructions = CASE WHEN sqlc.arg(set_instructions)::boolean THEN sqlc.arg(instructions) ELSE instructions END,
    runtime_id = CASE WHEN sqlc.arg(set_runtime_id)::boolean THEN sqlc.narg(runtime_id)::uuid ELSE runtime_id END,
    runtime_config = CASE WHEN sqlc.arg(set_runtime_config)::boolean THEN sqlc.arg(runtime_config)::jsonb ELSE runtime_config END,
    custom_args = CASE WHEN sqlc.arg(set_custom_args)::boolean THEN sqlc.arg(custom_args)::jsonb ELSE custom_args END,
    mcp_config = CASE WHEN sqlc.arg(set_mcp_config)::boolean THEN sqlc.arg(mcp_config)::jsonb ELSE mcp_config END,
    model = CASE WHEN sqlc.arg(set_model)::boolean THEN sqlc.narg(model)::text ELSE model END,
    thinking_level = CASE WHEN sqlc.arg(set_thinking_level)::boolean THEN sqlc.narg(thinking_level)::text ELSE thinking_level END,
    service_tier = CASE WHEN sqlc.arg(set_service_tier)::boolean THEN sqlc.narg(service_tier)::text ELSE service_tier END,
    max_concurrent_tasks = CASE WHEN sqlc.arg(set_max_concurrent_tasks)::boolean THEN sqlc.arg(max_concurrent_tasks) ELSE max_concurrent_tasks END,
    permission_mode = CASE WHEN sqlc.arg(set_permission_mode)::boolean THEN sqlc.arg(permission_mode) ELSE permission_mode END,
    disabled_runtime_skills = CASE WHEN sqlc.arg(set_disabled_runtime_skills)::boolean THEN sqlc.arg(disabled_runtime_skills)::jsonb ELSE disabled_runtime_skills END,
    updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: UpdateAgentCustomEnv :one
UPDATE agent
SET custom_env = sqlc.arg(custom_env), updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: ArchiveAgent :one
UPDATE agent
SET archived_at = now(), archived_by = sqlc.arg(actor_id), updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND archived_at IS NULL
RETURNING *;

-- name: RestoreAgent :one
UPDATE agent
SET archived_at = NULL, archived_by = NULL, updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND archived_at IS NOT NULL
RETURNING *;

-- name: CreateSkill :one
INSERT INTO skill (workspace_id, name, description, content, config, created_by)
VALUES (
    sqlc.arg(workspace_id), sqlc.arg(name), sqlc.arg(description),
    sqlc.arg(content), sqlc.arg(config), sqlc.arg(created_by)
)
RETURNING *;

-- name: GetSkill :one
SELECT * FROM skill WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: GetSkillByWorkspaceAndName :one
SELECT * FROM skill
WHERE workspace_id = $1 AND name = $2
LIMIT 1;

-- name: LockSkill :one
SELECT * FROM skill
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
FOR UPDATE;

-- name: ListSkills :many
SELECT * FROM skill
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY updated_at DESC, id DESC;

-- name: UpdateSkill :one
UPDATE skill
SET name = CASE WHEN sqlc.arg(set_name)::boolean THEN sqlc.arg(name) ELSE name END,
    description = CASE WHEN sqlc.arg(set_description)::boolean THEN sqlc.arg(description) ELSE description END,
    content = CASE WHEN sqlc.arg(set_content)::boolean THEN sqlc.arg(content) ELSE content END,
    config = CASE WHEN sqlc.arg(set_config)::boolean THEN sqlc.arg(config)::jsonb ELSE config END,
    updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: DeleteSkill :execrows
DELETE FROM skill WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: CountAgentSkillBindings :one
SELECT count(*) FROM agent_skill WHERE skill_id = sqlc.arg(skill_id);

-- name: UpsertSkillFile :one
INSERT INTO skill_file (skill_id, path, content)
VALUES (sqlc.arg(skill_id), sqlc.arg(path), sqlc.arg(content))
ON CONFLICT (skill_id, path) DO UPDATE
SET content = EXCLUDED.content, updated_at = now()
RETURNING *;

-- name: ListSkillFiles :many
SELECT * FROM skill_file WHERE skill_id = sqlc.arg(skill_id) ORDER BY path, id;

-- name: DeleteSkillFile :execrows
DELETE FROM skill_file WHERE id = sqlc.arg(id) AND skill_id = sqlc.arg(skill_id);

-- name: DeleteSkillFilesBySkill :execrows
DELETE FROM skill_file WHERE skill_id = sqlc.arg(skill_id);

-- name: ReplaceAgentSkill :one
INSERT INTO agent_skill (agent_id, skill_id, enabled)
VALUES (sqlc.arg(agent_id), sqlc.arg(skill_id), sqlc.arg(enabled))
ON CONFLICT (agent_id, skill_id) DO UPDATE SET enabled = EXCLUDED.enabled
RETURNING *;

-- name: ListAgentSkills :many
SELECT s.*, a_s.enabled
FROM agent_skill a_s
JOIN skill s ON s.id = a_s.skill_id
WHERE a_s.agent_id = sqlc.arg(agent_id)
ORDER BY s.name, s.id;

-- name: DeleteAgentSkills :execrows
DELETE FROM agent_skill WHERE agent_id = sqlc.arg(agent_id);

-- name: CreateAgentInvocationTarget :one
INSERT INTO agent_invocation_target (agent_id, target_type, target_id, created_by)
VALUES (sqlc.arg(agent_id), sqlc.arg(target_type), sqlc.arg(target_id), sqlc.arg(created_by))
ON CONFLICT (agent_id, target_type, target_id) DO UPDATE
SET created_by = EXCLUDED.created_by
RETURNING *;

-- name: ListAgentInvocationTargets :many
SELECT * FROM agent_invocation_target
WHERE agent_id = sqlc.arg(agent_id)
ORDER BY target_type, target_id;

-- name: DeleteAgentInvocationTargets :execrows
DELETE FROM agent_invocation_target WHERE agent_id = sqlc.arg(agent_id);
