-- name: CreateSquad :one
INSERT INTO squad (workspace_id, name, description, instructions, leader_id, creator_id)
VALUES (
    sqlc.arg(workspace_id), sqlc.arg(name), sqlc.arg(description),
    sqlc.arg(instructions), sqlc.arg(leader_id), sqlc.arg(creator_id)
)
RETURNING *;

-- name: GetSquad :one
SELECT * FROM squad WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: ListSquads :many
SELECT * FROM squad
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY archived_at NULLS FIRST, updated_at DESC, id DESC;

-- name: LockSquad :one
SELECT * FROM squad
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
FOR UPDATE;

-- name: UpdateSquad :one
UPDATE squad
SET name = CASE WHEN sqlc.arg(set_name)::boolean THEN sqlc.arg(name) ELSE name END,
    description = CASE WHEN sqlc.arg(set_description)::boolean THEN sqlc.arg(description) ELSE description END,
    instructions = CASE WHEN sqlc.arg(set_instructions)::boolean THEN sqlc.arg(instructions) ELSE instructions END,
    leader_id = CASE WHEN sqlc.arg(set_leader_id)::boolean THEN sqlc.arg(leader_id) ELSE leader_id END,
    updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: ArchiveSquad :one
UPDATE squad
SET archived_at = now(), archived_by = sqlc.arg(actor_id), updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND archived_at IS NULL
RETURNING *;

-- name: ReassignIssueAssigneesFromSquad :execrows
UPDATE issue
SET assignee_type = 'agent',
    assignee_id = sqlc.arg(leader_id),
    updated_at = now()
WHERE workspace_id = sqlc.arg(workspace_id)
  AND assignee_type = 'squad'
  AND assignee_id = sqlc.arg(squad_id);

-- name: UpsertSquadMember :one
INSERT INTO squad_member (squad_id, agent_id, role)
VALUES (sqlc.arg(squad_id), sqlc.arg(agent_id), sqlc.arg(role))
ON CONFLICT (squad_id, agent_id) DO UPDATE SET role = EXCLUDED.role
RETURNING *;

-- name: ListSquadMembers :many
SELECT sm.*, a.name AS agent_name, a.status AS agent_status, a.archived_at AS agent_archived_at
FROM squad_member sm
JOIN agent a ON a.id = sm.agent_id
WHERE sm.squad_id = sqlc.arg(squad_id)
ORDER BY (sm.role = 'leader') DESC, a.name, a.id;

-- name: DeleteSquadMember :execrows
DELETE FROM squad_member
WHERE squad_id = sqlc.arg(squad_id) AND agent_id = sqlc.arg(agent_id);

-- name: DemoteSquadLeadersExcept :execrows
UPDATE squad_member
SET role = 'member'
WHERE squad_id = sqlc.arg(squad_id)
  AND role = 'leader'
  AND agent_id <> sqlc.arg(leader_id);

-- name: CountActiveTasksBySquad :one
SELECT count(*)
FROM agent_task_queue
WHERE squad_id = sqlc.arg(squad_id)
  AND status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running');

-- name: ListSquadMemberStatuses :many
SELECT sm.agent_id,
       a.archived_at AS agent_archived_at,
       r.status AS runtime_status,
       r.last_seen_at AS runtime_last_seen_at,
       EXISTS (
           SELECT 1
           FROM agent_task_queue task
           WHERE task.agent_id = sm.agent_id
             AND task.workspace_id = a.workspace_id
             AND task.status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running')
       ) AS has_active_task,
       CAST((
           SELECT max(COALESCE(task.started_at, task.dispatched_at, task.created_at))
           FROM agent_task_queue task
           WHERE task.agent_id = sm.agent_id
             AND task.workspace_id = a.workspace_id
       ) AS timestamptz) AS last_active_at,
       (
           SELECT i.id
           FROM agent_task_queue task
           JOIN issue i ON i.id = task.issue_id AND i.workspace_id = task.workspace_id
           WHERE task.agent_id = sm.agent_id
             AND task.workspace_id = a.workspace_id
             AND task.issue_id IS NOT NULL
             AND task.status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running')
           ORDER BY task.created_at DESC
           LIMIT 1
       ) AS active_issue_id,
       CAST(COALESCE((
           SELECT i.title
           FROM agent_task_queue task
           JOIN issue i ON i.id = task.issue_id AND i.workspace_id = task.workspace_id
           WHERE task.agent_id = sm.agent_id
             AND task.workspace_id = a.workspace_id
             AND task.issue_id IS NOT NULL
             AND task.status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running')
           ORDER BY task.created_at DESC
           LIMIT 1
       ), '') AS text) AS active_issue_title
FROM squad_member sm
JOIN agent a ON a.id = sm.agent_id
LEFT JOIN agent_runtime r ON r.id = a.runtime_id AND r.workspace_id = a.workspace_id
WHERE sm.squad_id = sqlc.arg(squad_id)
ORDER BY (sm.role = 'leader') DESC, a.name, a.id;

-- name: CreateIssue :one
INSERT INTO issue (
    workspace_id, title, description, status, assignee_type, assignee_id,
    creator_type, creator_id, acceptance_criteria, context_refs, number,
    idempotency_key, request_hash
)
VALUES (
    sqlc.arg(workspace_id), sqlc.arg(title), sqlc.arg(description), sqlc.arg(status),
    sqlc.arg(assignee_type), sqlc.arg(assignee_id), sqlc.arg(creator_type),
    sqlc.arg(creator_id), sqlc.arg(acceptance_criteria), sqlc.arg(context_refs),
    sqlc.arg(number), sqlc.narg(idempotency_key), sqlc.narg(request_hash)
)
RETURNING *;

-- name: GetIssue :one
SELECT * FROM issue WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: LockIssue :one
SELECT * FROM issue
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
FOR UPDATE;

-- name: ListIssueGCStatuses :many
SELECT id, status, updated_at
FROM issue
WHERE workspace_id = sqlc.arg(workspace_id)
  AND id = ANY(sqlc.arg(issue_ids)::uuid[]);

-- name: GetIssueByIdempotencyKey :one
SELECT * FROM issue
WHERE workspace_id = sqlc.arg(workspace_id)
  AND creator_type = sqlc.arg(creator_type)
  AND creator_id = sqlc.arg(creator_id)
  AND idempotency_key = sqlc.arg(idempotency_key);

-- name: ListIssuesPage :many
SELECT *
FROM issue
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (
      sqlc.narg(cursor_updated_at)::timestamptz IS NULL
      OR (updated_at, id) < (sqlc.narg(cursor_updated_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
  )
  AND (cardinality(sqlc.arg(statuses)::text[]) = 0 OR status = ANY(sqlc.arg(statuses)::text[]))
  AND (sqlc.narg(assignee_type)::text IS NULL OR assignee_type = sqlc.narg(assignee_type)::text)
  AND (sqlc.narg(assignee_id)::uuid IS NULL OR assignee_id = sqlc.narg(assignee_id)::uuid)
  AND (sqlc.narg(creator_type)::text IS NULL OR creator_type = sqlc.narg(creator_type)::text)
  AND (sqlc.narg(creator_id)::uuid IS NULL OR creator_id = sqlc.narg(creator_id)::uuid)
ORDER BY updated_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: UpdateIssue :one
UPDATE issue
SET title = CASE WHEN sqlc.arg(set_title)::boolean THEN sqlc.arg(title) ELSE title END,
    description = CASE WHEN sqlc.arg(set_description)::boolean THEN sqlc.arg(description) ELSE description END,
    status = CASE WHEN sqlc.arg(set_status)::boolean THEN sqlc.arg(status) ELSE status END,
    assignee_type = CASE WHEN sqlc.arg(set_assignee_type)::boolean THEN sqlc.arg(assignee_type) ELSE assignee_type END,
    assignee_id = CASE WHEN sqlc.arg(set_assignee_id)::boolean THEN sqlc.arg(assignee_id)::uuid ELSE assignee_id END,
    acceptance_criteria = CASE WHEN sqlc.arg(set_acceptance_criteria)::boolean THEN sqlc.arg(acceptance_criteria)::jsonb ELSE acceptance_criteria END,
    context_refs = CASE WHEN sqlc.arg(set_context_refs)::boolean THEN sqlc.arg(context_refs)::jsonb ELSE context_refs END,
    updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: MarkIssueFirstExecuted :one
UPDATE issue
SET first_executed_at = now(), updated_at = now()
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND first_executed_at IS NULL
RETURNING *;

-- name: CreateInitialIssueTask :one
INSERT INTO agent_task_queue (
    workspace_id, agent_id, runtime_id, issue_id, squad_id, is_leader_task,
    status, context, initiator_user_id, originator_user_id, accountable_user_id,
    originator_source, trigger_evidence_kind, trigger_evidence_ref_id, tool_bundle_id
)
VALUES (
    sqlc.arg(workspace_id), sqlc.arg(agent_id), sqlc.arg(runtime_id), sqlc.arg(issue_id),
    sqlc.narg(squad_id), sqlc.arg(is_leader_task), 'queued', sqlc.arg(context),
    sqlc.arg(user_id), sqlc.arg(user_id), sqlc.arg(user_id), sqlc.arg(originator_source),
    'issue_assignment', sqlc.arg(issue_id),
    (SELECT head.bundle_id FROM agent_tool_bundle_head head
     WHERE head.workspace_id = sqlc.arg(workspace_id) AND head.agent_id = sqlc.arg(agent_id))
)
RETURNING *;

-- name: CountActiveTasksByIssue :one
SELECT count(*) FROM agent_task_queue
WHERE workspace_id = sqlc.arg(workspace_id)
  AND issue_id = sqlc.arg(issue_id)
  AND status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running');

-- name: TouchIssueForComment :one
UPDATE issue
SET updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: DeleteTaskTokensByIssue :execrows
DELETE FROM task_token WHERE task_id IN (
    SELECT task.id FROM agent_task_queue task
    WHERE task.workspace_id = sqlc.arg(target_workspace_id)
      AND task.issue_id = sqlc.arg(target_issue_id)
      AND task.status IN ('completed', 'failed', 'cancelled')
);

-- name: DeleteTaskMessagesByIssue :execrows
DELETE FROM task_message WHERE task_id IN (
    SELECT task.id FROM agent_task_queue task
    WHERE task.workspace_id = sqlc.arg(target_workspace_id)
      AND task.issue_id = sqlc.arg(target_issue_id)
      AND task.status IN ('completed', 'failed', 'cancelled')
);

-- name: DeleteTaskUsageByIssue :execrows
DELETE FROM task_usage WHERE task_id IN (
    SELECT task.id FROM agent_task_queue task
    WHERE task.workspace_id = sqlc.arg(target_workspace_id)
      AND task.issue_id = sqlc.arg(target_issue_id)
      AND task.status IN ('completed', 'failed', 'cancelled')
);

-- name: DeleteCommentsByIssue :execrows
DELETE FROM comment
WHERE workspace_id = sqlc.arg(workspace_id) AND issue_id = sqlc.arg(issue_id);

-- name: DeleteActivityByIssue :execrows
DELETE FROM activity_log
WHERE workspace_id = sqlc.arg(workspace_id) AND issue_id = sqlc.arg(issue_id);

-- name: DeleteTerminalTasksByIssue :execrows
DELETE FROM agent_task_queue
WHERE workspace_id = sqlc.arg(workspace_id)
  AND issue_id = sqlc.arg(issue_id)
  AND status IN ('completed', 'failed', 'cancelled');

-- name: DeleteIssue :execrows
DELETE FROM issue WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: CreateComment :one
INSERT INTO comment (
    workspace_id, issue_id, author_type, author_id, content, type,
    source_task_id, idempotency_key, request_hash
)
VALUES (
    sqlc.arg(workspace_id), sqlc.arg(issue_id), sqlc.arg(author_type),
    sqlc.narg(author_id), sqlc.arg(content), 'message', sqlc.narg(source_task_id),
    sqlc.narg(idempotency_key), sqlc.narg(request_hash)
)
RETURNING *;

-- name: GetCommentByIdempotencyKey :one
SELECT * FROM comment
WHERE workspace_id = sqlc.arg(workspace_id)
  AND issue_id = sqlc.arg(issue_id)
  AND author_type = sqlc.arg(author_type)
  AND author_id IS NOT DISTINCT FROM sqlc.narg(author_id)::uuid
  AND idempotency_key = sqlc.arg(idempotency_key);

-- name: ListCommentsPage :many
SELECT c.*,
       COALESCE(u.name, a.name, 'System') AS author_display
FROM comment c
LEFT JOIN "user" u ON c.author_type = 'member' AND u.id = c.author_id
LEFT JOIN agent a ON c.author_type = 'agent' AND a.id = c.author_id
WHERE c.workspace_id = sqlc.arg(workspace_id)
  AND c.issue_id = sqlc.arg(issue_id)
  AND (
      sqlc.narg(cursor_created_at)::timestamptz IS NULL
      OR (c.created_at, c.id) > (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
  )
ORDER BY c.created_at, c.id
LIMIT sqlc.arg(page_limit);

-- name: CreateAgentTask :one
INSERT INTO agent_task_queue (
    workspace_id, agent_id, runtime_id, issue_id, chat_session_id, squad_id,
    is_leader_task, status, priority, attempt, max_attempts, parent_task_id,
    context, trigger_summary, force_fresh_session, handoff_note, fire_at, runtime_mcp_overlay,
    initiator_user_id, originator_user_id, accountable_user_id, originator_source,
    trigger_comment_id, coalesced_comment_ids, chat_input_task_id,
    escalation_for_task_id, delegated_from_task_id, retry_of_task_id,
    rerun_of_task_id, trigger_evidence_kind, trigger_evidence_ref_id, tool_bundle_id
)
VALUES (
    sqlc.arg(workspace_id), sqlc.arg(agent_id), sqlc.arg(runtime_id),
    sqlc.narg(issue_id), sqlc.narg(chat_session_id), sqlc.narg(squad_id),
    sqlc.arg(is_leader_task), sqlc.arg(status), sqlc.arg(priority),
    sqlc.arg(attempt), sqlc.arg(max_attempts), sqlc.narg(parent_task_id),
    sqlc.arg(context), sqlc.narg(trigger_summary), sqlc.arg(force_fresh_session), sqlc.narg(handoff_note),
    sqlc.narg(fire_at), sqlc.arg(runtime_mcp_overlay), sqlc.narg(initiator_user_id),
    sqlc.narg(originator_user_id), sqlc.narg(accountable_user_id),
    sqlc.narg(originator_source), sqlc.narg(trigger_comment_id),
    sqlc.arg(coalesced_comment_ids), sqlc.narg(chat_input_task_id),
    sqlc.narg(escalation_for_task_id), sqlc.narg(delegated_from_task_id),
    sqlc.narg(retry_of_task_id), sqlc.narg(rerun_of_task_id),
    sqlc.narg(trigger_evidence_kind), sqlc.narg(trigger_evidence_ref_id),
    (SELECT head.bundle_id FROM agent_tool_bundle_head head
     WHERE head.workspace_id = sqlc.arg(workspace_id) AND head.agent_id = sqlc.arg(agent_id))
)
RETURNING *;

-- name: LockActiveTaskForIssueAgent :one
SELECT * FROM agent_task_queue
WHERE workspace_id = sqlc.arg(workspace_id)
  AND issue_id = sqlc.arg(issue_id)
  AND agent_id = sqlc.arg(agent_id)
  AND status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running')
ORDER BY CASE status
    WHEN 'queued' THEN 0
    WHEN 'deferred' THEN 1
    WHEN 'dispatched' THEN 2
    WHEN 'waiting_local_directory' THEN 3
    WHEN 'running' THEN 4
    ELSE 5
  END,
  created_at DESC, id DESC
LIMIT 1
FOR UPDATE;

-- name: AppendPlannedCommentToTask :one
UPDATE agent_task_queue
SET coalesced_comment_ids = CASE
    WHEN sqlc.arg(comment_id)::uuid = trigger_comment_id
      OR sqlc.arg(comment_id)::uuid = ANY(coalesced_comment_ids)
    THEN coalesced_comment_ids
    ELSE array_append(coalesced_comment_ids, sqlc.arg(comment_id)::uuid)
  END
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running')
RETURNING *;

-- name: GetAgentTask :one
SELECT * FROM agent_task_queue
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: LockAgentTask :one
SELECT * FROM agent_task_queue
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
FOR UPDATE;

-- name: ClaimTasksForRuntimes :many
WITH candidates AS (
	SELECT task.id
	FROM agent_task_queue task
	JOIN agent ON agent.id = task.agent_id
	WHERE task.workspace_id = sqlc.arg(workspace_id)
	  AND task.runtime_id = ANY(sqlc.arg(runtime_ids)::uuid[])
	  AND task.status = 'queued'
	  AND agent.workspace_id = task.workspace_id
	  AND agent.runtime_id = task.runtime_id
	  AND agent.archived_at IS NULL
	ORDER BY task.priority DESC, task.created_at, task.id
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(claim_limit)
)
UPDATE agent_task_queue task
SET status = 'dispatched',
    dispatched_at = COALESCE(task.dispatched_at, now()),
    prepare_lease_expires_at = sqlc.arg(prepare_lease_expires_at)
FROM candidates
WHERE task.id = candidates.id
RETURNING task.*;

-- name: SetTaskDeliveredComments :one
UPDATE agent_task_queue
SET delivered_comment_ids = (
    SELECT COALESCE(array_agg(DISTINCT planned.id), '{}')
    FROM unnest(array_append(coalesced_comment_ids, trigger_comment_id)) AS planned(id)
    WHERE planned.id IS NOT NULL
  )
WHERE id = sqlc.arg(id)
  AND runtime_id = sqlc.arg(runtime_id)
  AND status = 'dispatched'
RETURNING *;

-- name: ExtendTaskPrepareLease :one
UPDATE agent_task_queue
SET prepare_lease_expires_at = sqlc.arg(prepare_lease_expires_at)
WHERE id = sqlc.arg(id)
  AND runtime_id = sqlc.arg(runtime_id)
  AND status IN ('dispatched', 'waiting_local_directory')
RETURNING *;

-- name: MarkTaskWaitingLocalDirectory :one
UPDATE agent_task_queue
SET status = 'waiting_local_directory',
    wait_reason = sqlc.narg(wait_reason),
    prepare_lease_expires_at = sqlc.arg(prepare_lease_expires_at)
WHERE id = sqlc.arg(id) AND runtime_id = sqlc.arg(runtime_id) AND status = 'dispatched'
RETURNING *;

-- name: StartTask :one
UPDATE agent_task_queue
SET status = 'running', started_at = COALESCE(started_at, now()),
    wait_reason = NULL, prepare_lease_expires_at = NULL
WHERE id = sqlc.arg(id)
  AND runtime_id = sqlc.arg(runtime_id)
  AND status IN ('dispatched', 'waiting_local_directory')
RETURNING *;

-- name: UpdateTaskProgress :one
UPDATE agent_task_queue
SET trigger_summary = sqlc.arg(trigger_summary)
WHERE id = sqlc.arg(id) AND runtime_id = sqlc.arg(runtime_id) AND status = 'running'
RETURNING *;

-- name: UpdateTaskSession :one
UPDATE agent_task_queue
SET session_id = CASE WHEN sqlc.arg(set_session_id)::boolean THEN sqlc.narg(session_id)::text ELSE session_id END,
    work_dir = CASE WHEN sqlc.arg(set_work_dir)::boolean THEN sqlc.narg(work_dir)::text ELSE work_dir END
WHERE id = sqlc.arg(id) AND runtime_id = sqlc.arg(runtime_id)
  AND status IN ('dispatched', 'waiting_local_directory', 'running')
RETURNING *;

-- name: ListPendingTasksByRuntime :many
SELECT * FROM agent_task_queue
WHERE workspace_id = sqlc.arg(workspace_id)
  AND runtime_id = sqlc.arg(runtime_id)
  AND status IN ('dispatched', 'waiting_local_directory', 'running')
ORDER BY created_at, id;

-- name: RecoverOrphanedTasksForRuntime :many
UPDATE agent_task_queue
SET status = 'failed', error = 'runtime restarted before task completion',
    failure_reason = 'runtime_restart', completed_at = now(), prepare_lease_expires_at = NULL
WHERE workspace_id = sqlc.arg(workspace_id)
  AND runtime_id = sqlc.arg(runtime_id)
  AND status IN ('dispatched', 'waiting_local_directory', 'running')
RETURNING *;

-- name: FailActiveTasksForRuntime :many
UPDATE agent_task_queue
SET status = 'failed', error = sqlc.arg(error),
    failure_reason = sqlc.arg(failure_reason), completed_at = now(),
    prepare_lease_expires_at = NULL
WHERE workspace_id = sqlc.arg(workspace_id)
  AND runtime_id = sqlc.arg(runtime_id)
  AND status IN ('dispatched', 'waiting_local_directory', 'running')
RETURNING *;

-- name: ExpireStaleQueuedTasks :many
UPDATE agent_task_queue AS task
SET status = 'failed', error = 'task expired before it could be claimed',
    failure_reason = 'queue_expired', completed_at = now()
WHERE task.id IN (
    SELECT candidate.id FROM agent_task_queue AS candidate
    WHERE candidate.status = 'queued' AND candidate.created_at <= sqlc.arg(expire_before)
    ORDER BY candidate.created_at, candidate.id
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(batch_limit)
)
RETURNING task.*;

-- name: PromoteDeferredTasks :many
UPDATE agent_task_queue
SET status = 'queued', fire_at = NULL
WHERE id IN (
    SELECT id FROM agent_task_queue
    WHERE status = 'deferred' AND fire_at <= now()
    ORDER BY fire_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(batch_limit)
)
RETURNING *;

-- name: RequeueExpiredPrepareLeases :many
UPDATE agent_task_queue
SET status = 'queued', dispatched_at = NULL, wait_reason = NULL,
    prepare_lease_expires_at = NULL
WHERE id IN (
    SELECT id FROM agent_task_queue
    WHERE status IN ('dispatched', 'waiting_local_directory')
      AND prepare_lease_expires_at <= now()
    ORDER BY prepare_lease_expires_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(batch_limit)
)
RETURNING *;

-- name: CompleteTask :one
UPDATE agent_task_queue
SET status = 'completed', result = sqlc.arg(result), error = NULL,
    session_id = CASE WHEN sqlc.arg(session_rollout_missing)::boolean THEN NULL ELSE sqlc.narg(session_id)::text END,
    work_dir = sqlc.narg(work_dir),
    session_rollout_missing = sqlc.arg(session_rollout_missing),
    retired_session_id = sqlc.narg(retired_session_id),
    completed_at = COALESCE(completed_at, now())
WHERE id = sqlc.arg(id) AND runtime_id = sqlc.arg(runtime_id)
  AND status IN ('running', 'completed')
RETURNING *;

-- name: FailTask :one
UPDATE agent_task_queue
SET status = 'failed', error = sqlc.arg(error), failure_reason = sqlc.narg(failure_reason),
    session_id = CASE WHEN sqlc.arg(session_rollout_missing)::boolean THEN NULL ELSE sqlc.narg(session_id)::text END,
    work_dir = sqlc.narg(work_dir),
    session_rollout_missing = sqlc.arg(session_rollout_missing),
    retired_session_id = sqlc.narg(retired_session_id),
    completed_at = COALESCE(completed_at, now())
WHERE id = sqlc.arg(id) AND runtime_id = sqlc.arg(runtime_id)
  AND status IN ('dispatched', 'waiting_local_directory', 'running', 'failed')
RETURNING *;

-- name: CreateRetryTask :one
INSERT INTO agent_task_queue (
    workspace_id, agent_id, runtime_id, issue_id, chat_session_id, squad_id,
    is_leader_task, status, priority, attempt, max_attempts, parent_task_id,
    session_id, work_dir, context, trigger_summary, force_fresh_session,
    handoff_note, fire_at, runtime_mcp_overlay, initiator_user_id,
    originator_user_id, accountable_user_id, originator_source,
    trigger_comment_id, coalesced_comment_ids, delivered_comment_ids,
    chat_input_task_id, escalation_for_task_id, delegated_from_task_id,
    retry_of_task_id, trigger_evidence_kind, trigger_evidence_ref_id,
    session_rollout_missing, retired_session_id, tool_bundle_id
)
SELECT
    parent.workspace_id, parent.agent_id, parent.runtime_id, parent.issue_id,
    parent.chat_session_id, parent.squad_id, parent.is_leader_task,
    sqlc.arg(retry_status), parent.priority, parent.attempt + 1,
    parent.max_attempts, COALESCE(parent.parent_task_id, parent.id),
    CASE WHEN sqlc.arg(clear_session)::boolean THEN NULL ELSE parent.session_id END,
    parent.work_dir, parent.context, parent.trigger_summary,
    CASE WHEN sqlc.arg(clear_session)::boolean THEN true ELSE parent.force_fresh_session END,
    parent.handoff_note, sqlc.narg(fire_at), parent.runtime_mcp_overlay,
    parent.initiator_user_id, parent.originator_user_id, parent.accountable_user_id,
    parent.originator_source, parent.trigger_comment_id,
    parent.coalesced_comment_ids, parent.delivered_comment_ids,
    parent.chat_input_task_id, parent.escalation_for_task_id,
    parent.delegated_from_task_id, parent.id, parent.trigger_evidence_kind,
    parent.trigger_evidence_ref_id, parent.session_rollout_missing,
    parent.retired_session_id, parent.tool_bundle_id
FROM agent_task_queue parent
WHERE parent.id = sqlc.arg(parent_task_id)
  AND parent.workspace_id = sqlc.arg(workspace_id)
  AND parent.status = 'failed'
  AND parent.attempt < parent.max_attempts
  AND NOT EXISTS (
      SELECT 1 FROM agent_task_queue child
      WHERE child.workspace_id = parent.workspace_id AND child.retry_of_task_id = parent.id
  )
RETURNING *;

-- name: CancelTask :one
UPDATE agent_task_queue
SET status = 'cancelled', completed_at = COALESCE(completed_at, now()),
    chat_finalize_deferred_at = CASE
        WHEN chat_session_id IS NOT NULL AND status IN ('dispatched', 'waiting_local_directory', 'running') THEN now()
        ELSE NULL
    END,
    prepare_lease_expires_at = NULL
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running', 'cancelled')
RETURNING *;

-- name: FinalizeCancelledTask :one
UPDATE agent_task_queue
SET chat_finalize_deferred_at = NULL
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
  AND status = 'cancelled'
RETURNING *;

-- name: ListDeferredCancelledTasks :many
SELECT * FROM agent_task_queue
WHERE status = 'cancelled'
  AND chat_finalize_deferred_at IS NOT NULL
  AND chat_finalize_deferred_at <= now() - make_interval(secs => sqlc.arg(grace_seconds)::double precision)
ORDER BY chat_finalize_deferred_at, id
LIMIT sqlc.arg(batch_limit)
FOR UPDATE SKIP LOCKED;

-- name: ListTaskRunsPage :many
SELECT task.*,
       COALESCE(agent.name, '[deleted agent]') AS agent_name,
       COALESCE(runtime.custom_name, runtime.name, '[deleted runtime]') AS runtime_name,
       COALESCE(squad.name, '[deleted squad]') AS squad_name
FROM agent_task_queue task
LEFT JOIN agent ON agent.id = task.agent_id
LEFT JOIN agent_runtime runtime ON runtime.id = task.runtime_id
LEFT JOIN squad ON squad.id = task.squad_id
WHERE task.workspace_id = sqlc.arg(workspace_id)
  AND task.issue_id = sqlc.arg(issue_id)
  AND (
      sqlc.narg(cursor_created_at)::timestamptz IS NULL
      OR (task.created_at, task.id) > (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
  )
ORDER BY task.created_at, task.id
LIMIT sqlc.arg(page_limit);

-- name: ListActiveTaskRuns :many
SELECT task.*,
       COALESCE(agent.name, '[deleted agent]') AS agent_name,
       COALESCE(runtime.custom_name, runtime.name, '[deleted runtime]') AS runtime_name,
       COALESCE(squad.name, '[deleted squad]') AS squad_name
FROM agent_task_queue task
LEFT JOIN agent ON agent.id = task.agent_id
LEFT JOIN agent_runtime runtime ON runtime.id = task.runtime_id
LEFT JOIN squad ON squad.id = task.squad_id
WHERE task.workspace_id = sqlc.arg(workspace_id)
  AND task.issue_id = sqlc.arg(issue_id)
  AND task.status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running')
ORDER BY task.created_at, task.id;

-- name: CreateTaskMessage :one
INSERT INTO task_message (task_id, seq, type, tool, content, input, output)
VALUES (
    sqlc.arg(task_id), sqlc.arg(seq), sqlc.arg(type), sqlc.narg(tool),
    sqlc.narg(content), sqlc.narg(input), sqlc.narg(output)
)
ON CONFLICT (task_id, seq) DO NOTHING
RETURNING *;

-- name: GetTaskMessageBySeq :one
SELECT * FROM task_message
WHERE task_id = sqlc.arg(task_id) AND seq = sqlc.arg(seq);

-- name: ListTaskMessagesPage :many
SELECT * FROM task_message
WHERE task_id = sqlc.arg(task_id)
  AND (
      sqlc.narg(cursor_created_at)::timestamptz IS NULL
      OR (created_at, id) > (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
  )
ORDER BY created_at, id
LIMIT sqlc.arg(page_limit);

-- name: ListTaskMessagesSince :many
SELECT * FROM task_message
WHERE task_id = sqlc.arg(task_id) AND seq > sqlc.arg(after_seq)
ORDER BY seq, id;

-- name: UpsertTaskUsage :one
INSERT INTO task_usage (
    task_id, provider, model, input_tokens, output_tokens,
    cache_read_tokens, cache_write_tokens, cost_usd_ticks
)
VALUES (
    sqlc.arg(task_id), sqlc.arg(provider), sqlc.arg(model), sqlc.arg(input_tokens),
    sqlc.arg(output_tokens), sqlc.arg(cache_read_tokens), sqlc.arg(cache_write_tokens),
    sqlc.narg(cost_usd_ticks)
)
ON CONFLICT (task_id, provider, model) DO UPDATE
SET input_tokens = EXCLUDED.input_tokens,
    output_tokens = EXCLUDED.output_tokens,
    cache_read_tokens = EXCLUDED.cache_read_tokens,
    cache_write_tokens = EXCLUDED.cache_write_tokens,
    cost_usd_ticks = EXCLUDED.cost_usd_ticks,
    updated_at = now()
RETURNING *;

-- name: ListTaskUsageForTasks :many
SELECT * FROM task_usage
WHERE task_id = ANY(sqlc.arg(task_ids)::uuid[])
ORDER BY task_id, provider, model, id;

-- name: CreateActivity :one
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
VALUES (
    sqlc.arg(workspace_id), sqlc.narg(issue_id), sqlc.arg(actor_type),
    sqlc.narg(actor_id), sqlc.arg(action), sqlc.arg(details)
)
RETURNING *;

-- name: GetSquadEvaluationByTask :one
SELECT * FROM activity_log
WHERE workspace_id = sqlc.arg(workspace_id)
  AND issue_id = sqlc.arg(issue_id)
  AND action = 'squad_leader_evaluated'
  AND details ->> 'task_id' = sqlc.arg(task_id)::uuid::text
ORDER BY created_at, id
LIMIT 1;
