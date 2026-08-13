-- name: CountActiveTasksByWorkspace :one
SELECT count(*) FROM agent_task_queue
WHERE workspace_id = sqlc.arg(workspace_id)
  AND status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running');

-- name: CountOnlineRuntimesByWorkspace :one
SELECT count(*) FROM agent_runtime
WHERE workspace_id = sqlc.arg(workspace_id) AND status = 'online';

-- name: DeleteActivityByWorkspace :execrows
DELETE FROM activity_log WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteTaskUsageByWorkspace :execrows
DELETE FROM task_usage WHERE task_id IN (
    SELECT id FROM agent_task_queue WHERE workspace_id = sqlc.arg(workspace_id)
);

-- name: DeleteTaskMessagesByWorkspace :execrows
DELETE FROM task_message WHERE task_id IN (
    SELECT id FROM agent_task_queue WHERE workspace_id = sqlc.arg(workspace_id)
);

-- name: DeleteChatDraftRestoresByWorkspace :execrows
DELETE FROM chat_draft_restore WHERE chat_session_id IN (
    SELECT id FROM chat_session WHERE workspace_id = sqlc.arg(workspace_id)
);

-- name: DeleteChatMessagesByWorkspace :execrows
DELETE FROM chat_message WHERE chat_session_id IN (
    SELECT id FROM chat_session WHERE workspace_id = sqlc.arg(workspace_id)
);

-- name: DeleteTasksByWorkspace :execrows
DELETE FROM agent_task_queue WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteAgentToolBundleHeadsByWorkspace :execrows
DELETE FROM agent_tool_bundle_head WHERE workspace_id = sqlc.arg(workspace_id);

-- name: RevokeToolBundlesByWorkspace :execrows
UPDATE tool_bundle
SET status = 'revoked', revoked_at = COALESCE(revoked_at, now())
WHERE workspace_id = sqlc.arg(workspace_id) AND status = 'active';

-- name: DeleteToolBundleItemsByWorkspace :execrows
DELETE FROM tool_bundle_item WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteToolBundlesByWorkspace :execrows
DELETE FROM tool_bundle WHERE workspace_id = sqlc.arg(workspace_id);

-- name: ClearToolSourceCurrentRevisionsByWorkspace :execrows
UPDATE tool_source
SET enabled = false, current_revision = NULL, updated_at = now()
WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteToolDefinitionsByWorkspace :execrows
DELETE FROM tool_definition WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteToolSourceRevisionsByWorkspace :execrows
DELETE FROM tool_source_revision WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteToolSourceArtifactsByWorkspace :execrows
DELETE FROM tool_source_artifact WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteToolSourceSecretsByWorkspace :execrows
DELETE FROM tool_source_secret WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteToolSourcesByWorkspace :execrows
DELETE FROM tool_source WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteChatSessionsByWorkspace :execrows
DELETE FROM chat_session WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteCommentsByWorkspace :execrows
DELETE FROM comment WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteIssuesByWorkspace :execrows
DELETE FROM issue WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteSquadMembersByWorkspace :execrows
DELETE FROM squad_member WHERE squad_id IN (
    SELECT id FROM squad WHERE workspace_id = sqlc.arg(workspace_id)
);

-- name: DeleteSquadsByWorkspace :execrows
DELETE FROM squad WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteAgentInvocationTargetsByWorkspace :execrows
DELETE FROM agent_invocation_target WHERE agent_id IN (
    SELECT id FROM agent WHERE workspace_id = sqlc.arg(workspace_id)
);

-- name: DeleteAgentSkillsByWorkspace :execrows
DELETE FROM agent_skill WHERE agent_id IN (
    SELECT id FROM agent WHERE workspace_id = sqlc.arg(workspace_id)
);

-- name: DeleteSkillFilesByWorkspace :execrows
DELETE FROM skill_file WHERE skill_id IN (
    SELECT id FROM skill WHERE workspace_id = sqlc.arg(workspace_id)
);

-- name: DeleteSkillsByWorkspace :execrows
DELETE FROM skill WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteAgentsByWorkspace :execrows
DELETE FROM agent WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteAgentRuntimesByWorkspace :execrows
DELETE FROM agent_runtime WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteRuntimeProfilesByWorkspace :execrows
DELETE FROM runtime_profile WHERE workspace_id = sqlc.arg(workspace_id);
