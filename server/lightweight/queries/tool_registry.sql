-- name: CreateToolSource :one
INSERT INTO tool_source (workspace_id, name, kind, created_by)
VALUES (sqlc.arg(workspace_id), sqlc.arg(name), sqlc.arg(kind), sqlc.arg(created_by))
RETURNING *;

-- name: GetToolSource :one
SELECT * FROM tool_source
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: GetToolSourceByName :one
SELECT * FROM tool_source
WHERE workspace_id = sqlc.arg(workspace_id) AND name = sqlc.arg(name);

-- name: ListToolSources :many
SELECT * FROM tool_source
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY updated_at DESC, id
LIMIT sqlc.arg(page_limit);

-- name: UpdateToolSource :one
UPDATE tool_source
SET name = sqlc.arg(name), updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: SetToolSourceEnabled :one
UPDATE tool_source
SET enabled = sqlc.arg(enabled), updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: AdvanceToolSourceCurrentRevision :one
UPDATE tool_source source
SET current_revision = revision.id, updated_at = now()
FROM tool_source_revision revision
WHERE source.id = sqlc.arg(source_id)
  AND source.workspace_id = sqlc.arg(workspace_id)
  AND revision.id = sqlc.arg(revision_id)
  AND revision.workspace_id = source.workspace_id
  AND revision.source_id = source.id
  AND revision.status = 'ready'
RETURNING source.*;

-- name: CreateToolSourceRevision :one
WITH locked_source AS (
    SELECT id, workspace_id
    FROM tool_source
    WHERE id = sqlc.arg(source_id) AND workspace_id = sqlc.arg(workspace_id)
    FOR UPDATE
), next_revision AS (
    SELECT COALESCE(max(revision.revision), 0) + 1 AS revision
    FROM locked_source source
    LEFT JOIN tool_source_revision revision
      ON revision.source_id = source.id AND revision.workspace_id = source.workspace_id
)
INSERT INTO tool_source_revision (
    id, workspace_id, source_id, revision, endpoint, transport_config,
    artifact_id, secret_id, created_by
)
SELECT
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(source_id), next_revision.revision,
    sqlc.narg(endpoint), sqlc.arg(transport_config), sqlc.narg(artifact_id),
    sqlc.narg(secret_id), sqlc.arg(created_by)
FROM locked_source, next_revision
RETURNING *;

-- name: GetToolSourceRevision :one
SELECT * FROM tool_source_revision
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND source_id = sqlc.arg(source_id);

-- name: ListToolSourceRevisions :many
SELECT * FROM tool_source_revision
WHERE workspace_id = sqlc.arg(workspace_id) AND source_id = sqlc.arg(source_id)
ORDER BY revision DESC;

-- name: MarkToolSourceRevisionReady :one
UPDATE tool_source_revision
SET status = 'ready', validation_code = NULL, published_at = now()
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND source_id = sqlc.arg(source_id)
  AND status = 'validating'
RETURNING *;

-- name: MarkToolSourceRevisionFailed :one
UPDATE tool_source_revision
SET status = 'failed', validation_code = sqlc.arg(validation_code), published_at = NULL
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND source_id = sqlc.arg(source_id)
  AND status = 'validating'
RETURNING *;

-- name: RetireToolSourceRevision :one
UPDATE tool_source_revision
SET status = 'retired'
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND source_id = sqlc.arg(source_id)
  AND status = 'ready'
RETURNING *;

-- name: CreateToolDefinition :one
INSERT INTO tool_definition (
    workspace_id, source_id, source_revision_id, public_name, upstream_name,
    description, input_schema, output_schema, operation_metadata
)
VALUES (
    sqlc.arg(workspace_id), sqlc.arg(source_id), sqlc.arg(source_revision_id),
    sqlc.arg(public_name), sqlc.arg(upstream_name), sqlc.arg(description),
    sqlc.arg(input_schema), sqlc.narg(output_schema), sqlc.arg(operation_metadata)
)
RETURNING *;

-- name: ListToolDefinitionsByRevision :many
SELECT * FROM tool_definition
WHERE workspace_id = sqlc.arg(workspace_id)
  AND source_id = sqlc.arg(source_id)
  AND source_revision_id = sqlc.arg(source_revision_id)
ORDER BY public_name, id;

-- name: GetBundlePublicationToolDefinition :one
SELECT
    definition.id,
    definition.workspace_id,
    definition.source_id,
    definition.source_revision_id,
    definition.public_name,
    definition.upstream_name,
    definition.description,
    definition.input_schema,
    definition.output_schema,
    definition.operation_metadata,
    definition.enabled,
    definition.created_at,
    source.kind AS source_kind,
    revision.artifact_id,
    revision.endpoint,
    revision.transport_config
FROM tool_definition definition
JOIN tool_source source
  ON source.id = definition.source_id AND source.workspace_id = definition.workspace_id
JOIN tool_source_revision revision
  ON revision.id = definition.source_revision_id
 AND revision.workspace_id = definition.workspace_id
 AND revision.source_id = definition.source_id
WHERE definition.id = sqlc.arg(id)
  AND definition.workspace_id = sqlc.arg(workspace_id)
  AND definition.enabled = true
  AND source.enabled = true
  AND source.current_revision = definition.source_revision_id
  AND revision.status = 'ready';

-- name: SetToolDefinitionEnabled :one
UPDATE tool_definition
SET enabled = sqlc.arg(enabled)
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND source_id = sqlc.arg(source_id)
RETURNING *;

-- name: CreateToolSourceArtifact :one
WITH inserted AS (
    INSERT INTO tool_source_artifact (
        workspace_id, source_id, sha256, media_type, size_bytes, content
    )
    VALUES (
        sqlc.arg(workspace_id), sqlc.arg(source_id), sqlc.arg(sha256),
        sqlc.arg(media_type), sqlc.arg(size_bytes), sqlc.arg(content)
    )
    ON CONFLICT (workspace_id, source_id, sha256) DO NOTHING
    RETURNING *
)
SELECT * FROM inserted
UNION ALL
SELECT * FROM tool_source_artifact
WHERE workspace_id = sqlc.arg(workspace_id)
  AND source_id = sqlc.arg(source_id)
  AND sha256 = sqlc.arg(sha256)
LIMIT 1;

-- name: GetToolSourceArtifact :one
SELECT id, workspace_id, source_id, sha256, media_type, size_bytes, content, created_at
FROM tool_source_artifact
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND source_id = sqlc.arg(source_id);

-- name: GetToolSourceArtifactMetadata :one
SELECT id, workspace_id, source_id, sha256, media_type, size_bytes, created_at
FROM tool_source_artifact
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND source_id = sqlc.arg(source_id);

-- name: CreateToolSourceSecret :one
INSERT INTO tool_source_secret (id, workspace_id, source_id, envelope, key_id)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(source_id), sqlc.arg(envelope), sqlc.arg(key_id))
RETURNING *;

-- name: GetToolSourceSecret :one
SELECT * FROM tool_source_secret
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND source_id = sqlc.arg(source_id);

-- name: UpdateToolSourceSecret :one
UPDATE tool_source_secret
SET envelope = sqlc.arg(envelope), key_id = sqlc.arg(key_id), updated_at = now()
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND source_id = sqlc.arg(source_id)
RETURNING *;

-- name: GetToolSourceSecretMetadata :one
SELECT id, workspace_id, source_id, key_id, created_at, updated_at
FROM tool_source_secret
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND source_id = sqlc.arg(source_id);

-- name: CreateToolBundle :one
INSERT INTO tool_bundle (id, workspace_id, manifest_hash, created_by)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(manifest_hash), sqlc.arg(created_by))
RETURNING *;

-- name: CreateToolBundleItem :one
INSERT INTO tool_bundle_item (
    workspace_id, bundle_id, ordinal, public_name, source_id,
    source_revision_id, tool_definition_id, artifact_id,
    definition_snapshot, invocation_plan
)
VALUES (
    sqlc.arg(workspace_id), sqlc.arg(bundle_id), sqlc.arg(ordinal),
    sqlc.arg(public_name), sqlc.arg(source_id), sqlc.arg(source_revision_id),
    sqlc.arg(tool_definition_id), sqlc.narg(artifact_id),
    sqlc.arg(definition_snapshot), sqlc.arg(invocation_plan)
)
RETURNING *;

-- name: UpsertAgentToolBundleHead :one
INSERT INTO agent_tool_bundle_head (workspace_id, agent_id, bundle_id, updated_by)
VALUES (sqlc.arg(workspace_id), sqlc.arg(agent_id), sqlc.arg(bundle_id), sqlc.arg(updated_by))
ON CONFLICT (workspace_id, agent_id) DO UPDATE
SET bundle_id = EXCLUDED.bundle_id, updated_by = EXCLUDED.updated_by, updated_at = now()
RETURNING *;

-- name: GetAgentToolBundleHead :one
SELECT * FROM agent_tool_bundle_head
WHERE workspace_id = sqlc.arg(workspace_id) AND agent_id = sqlc.arg(agent_id);

-- name: DeleteAgentToolBundleHead :execrows
DELETE FROM agent_tool_bundle_head
WHERE workspace_id = sqlc.arg(workspace_id) AND agent_id = sqlc.arg(agent_id);

-- name: GetToolBundle :one
SELECT * FROM tool_bundle
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: ListToolBundleItems :many
SELECT * FROM tool_bundle_item
WHERE workspace_id = sqlc.arg(workspace_id) AND bundle_id = sqlc.arg(bundle_id)
ORDER BY ordinal, id;

-- name: RevokeToolBundle :one
UPDATE tool_bundle
SET status = 'revoked', revoked_at = now()
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND status = 'active'
RETURNING *;

-- name: PinAgentTaskToolBundle :execrows
UPDATE agent_task_queue task
SET tool_bundle_id = sqlc.arg(bundle_id)
WHERE task.id = sqlc.arg(task_id)
  AND task.workspace_id = sqlc.arg(workspace_id)
  AND task.agent_id = sqlc.arg(agent_id)
  AND task.tool_bundle_id IS NULL
  AND EXISTS (
      SELECT 1 FROM tool_bundle bundle
      WHERE bundle.id = sqlc.arg(bundle_id)
        AND bundle.workspace_id = task.workspace_id
        AND bundle.status = 'active'
  );

-- name: GetTaskBundleClaim :one
SELECT
    task.id AS task_id,
    task.workspace_id,
    task.agent_id,
    task.tool_bundle_id,
    bundle.status AS bundle_status
FROM agent_task_queue task
JOIN tool_bundle bundle
  ON bundle.id = task.tool_bundle_id AND bundle.workspace_id = task.workspace_id
WHERE task.id = sqlc.arg(task_id)
  AND task.workspace_id = sqlc.arg(workspace_id)
  AND task.agent_id = sqlc.arg(agent_id)
  AND task.tool_bundle_id IS NOT NULL;

-- name: GetAuthorizedTaskBundle :one
SELECT bundle.*
FROM agent_task_queue task
JOIN agent ON agent.id = task.agent_id AND agent.workspace_id = task.workspace_id
JOIN tool_bundle bundle
  ON bundle.id = task.tool_bundle_id AND bundle.workspace_id = task.workspace_id
WHERE task.id = sqlc.arg(task_id)
  AND task.workspace_id = sqlc.arg(workspace_id)
  AND task.agent_id = sqlc.arg(agent_id)
  AND task.tool_bundle_id = sqlc.arg(bundle_id)
  AND task.status IN ('dispatched', 'waiting_local_directory', 'running')
  AND agent.archived_at IS NULL
  AND bundle.status = 'active';

-- name: ListAuthorizedTaskBundleItems :many
SELECT item.*
FROM agent_task_queue task
JOIN agent ON agent.id = task.agent_id AND agent.workspace_id = task.workspace_id
JOIN tool_bundle bundle
  ON bundle.id = task.tool_bundle_id AND bundle.workspace_id = task.workspace_id
JOIN tool_bundle_item item
  ON item.bundle_id = bundle.id AND item.workspace_id = bundle.workspace_id
JOIN tool_source source
  ON source.id = item.source_id AND source.workspace_id = item.workspace_id
JOIN tool_definition definition
  ON definition.id = item.tool_definition_id AND definition.workspace_id = item.workspace_id
WHERE task.id = sqlc.arg(task_id)
  AND task.workspace_id = sqlc.arg(workspace_id)
  AND task.agent_id = sqlc.arg(agent_id)
  AND task.tool_bundle_id = sqlc.arg(bundle_id)
  AND task.status IN ('dispatched', 'waiting_local_directory', 'running')
  AND agent.archived_at IS NULL
  AND bundle.status = 'active'
  AND source.enabled = true
  AND definition.enabled = true
ORDER BY item.ordinal, item.id;

-- name: GetAuthorizedTaskBundleItemByName :one
SELECT item.*
FROM agent_task_queue task
JOIN agent ON agent.id = task.agent_id AND agent.workspace_id = task.workspace_id
JOIN tool_bundle bundle
  ON bundle.id = task.tool_bundle_id AND bundle.workspace_id = task.workspace_id
JOIN tool_bundle_item item
  ON item.bundle_id = bundle.id AND item.workspace_id = bundle.workspace_id
JOIN tool_source source
  ON source.id = item.source_id AND source.workspace_id = item.workspace_id
JOIN tool_definition definition
  ON definition.id = item.tool_definition_id AND definition.workspace_id = item.workspace_id
WHERE task.id = sqlc.arg(task_id)
  AND task.workspace_id = sqlc.arg(workspace_id)
  AND task.agent_id = sqlc.arg(agent_id)
  AND task.tool_bundle_id = sqlc.arg(bundle_id)
  AND item.public_name = sqlc.arg(public_name)
  AND task.status IN ('dispatched', 'waiting_local_directory', 'running')
  AND agent.archived_at IS NULL
  AND bundle.status = 'active'
  AND source.enabled = true
  AND definition.enabled = true;

-- name: GetTaskTokenBundleScope :one
SELECT token.id, token.task_id, token.agent_id, token.workspace_id, token.user_id,
       token.tool_bundle_id, token.expires_at
FROM task_token token
JOIN agent_task_queue task
  ON task.id = token.task_id
 AND task.workspace_id = token.workspace_id
 AND task.agent_id = token.agent_id
 AND task.tool_bundle_id IS NOT DISTINCT FROM token.tool_bundle_id
WHERE token.token_hash = sqlc.arg(token_hash)
  AND token.expires_at > now();

-- name: CountToolSourceBundleRetainers :one
SELECT count(*) FROM tool_bundle_item
WHERE workspace_id = sqlc.arg(workspace_id) AND source_id = sqlc.arg(source_id);

-- name: ClearToolSourceCurrentRevision :one
UPDATE tool_source
SET enabled = false, current_revision = NULL, updated_at = now()
WHERE id = sqlc.arg(source_id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: DeleteToolDefinitionsBySource :execrows
DELETE FROM tool_definition
WHERE workspace_id = sqlc.arg(workspace_id) AND source_id = sqlc.arg(source_id);

-- name: DeleteToolSourceRevisionsBySource :execrows
DELETE FROM tool_source_revision
WHERE workspace_id = sqlc.arg(workspace_id) AND source_id = sqlc.arg(source_id);

-- name: DeleteToolSourceArtifactsBySource :execrows
DELETE FROM tool_source_artifact
WHERE workspace_id = sqlc.arg(workspace_id) AND source_id = sqlc.arg(source_id);

-- name: DeleteToolSourceSecretsBySource :execrows
DELETE FROM tool_source_secret
WHERE workspace_id = sqlc.arg(workspace_id) AND source_id = sqlc.arg(source_id);

-- name: DeleteToolSource :execrows
DELETE FROM tool_source
WHERE id = sqlc.arg(source_id) AND workspace_id = sqlc.arg(workspace_id);

-- name: CountToolBundleRetainers :one
SELECT
    (SELECT count(*) FROM agent_tool_bundle_head head
     WHERE head.workspace_id = sqlc.arg(workspace_id) AND head.bundle_id = sqlc.arg(bundle_id))
  + (SELECT count(*) FROM agent_task_queue task
     WHERE task.workspace_id = sqlc.arg(workspace_id) AND task.tool_bundle_id = sqlc.arg(bundle_id))
  + (SELECT count(*) FROM task_token token
     WHERE token.workspace_id = sqlc.arg(workspace_id) AND token.tool_bundle_id = sqlc.arg(bundle_id)) AS retainers;

-- name: DeleteToolBundleItems :execrows
DELETE FROM tool_bundle_item
WHERE workspace_id = sqlc.arg(workspace_id) AND bundle_id = sqlc.arg(bundle_id);

-- name: DeleteToolBundle :execrows
DELETE FROM tool_bundle
WHERE workspace_id = sqlc.arg(workspace_id)
  AND id = sqlc.arg(bundle_id)
  AND status = 'revoked';
