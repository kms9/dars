-- name: GetSchemaMetadata :one
SELECT id, edition, schema_version, installed_at, updated_at
FROM schema_metadata
WHERE id = 1;

-- name: CreateUser :one
INSERT INTO "user" (name, email, language, timezone)
VALUES (sqlc.arg(name), sqlc.arg(email), sqlc.arg(language), sqlc.narg(timezone))
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM "user" WHERE id = sqlc.arg(id);

-- name: GetUserByEmail :one
SELECT * FROM "user" WHERE email = sqlc.arg(email);

-- name: UpdateUserProfile :one
UPDATE "user"
SET name = CASE WHEN sqlc.arg(set_name)::boolean THEN sqlc.arg(name) ELSE name END,
    language = CASE WHEN sqlc.arg(set_language)::boolean THEN sqlc.arg(language) ELSE language END,
    timezone = CASE WHEN sqlc.arg(set_timezone)::boolean THEN sqlc.narg(timezone)::text ELSE timezone END,
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: CreateVerificationCode :one
INSERT INTO verification_code (email, code_hash, expires_at)
VALUES (sqlc.arg(email), sqlc.arg(code_hash), sqlc.arg(expires_at))
RETURNING id, email, code_hash, expires_at, used_at, attempts, created_at;

-- name: GetLatestUsableVerificationCode :one
SELECT *
FROM verification_code
WHERE email = sqlc.arg(email)
  AND used_at IS NULL
  AND attempts < 5
  AND expires_at > now()
ORDER BY created_at DESC
LIMIT 1
FOR UPDATE;

-- name: IncrementVerificationCodeAttempts :one
UPDATE verification_code
SET attempts = attempts + 1
WHERE id = sqlc.arg(id) AND used_at IS NULL AND attempts < 5
RETURNING attempts;

-- name: ConsumeVerificationCode :execrows
UPDATE verification_code
SET used_at = now()
WHERE id = sqlc.arg(id) AND used_at IS NULL AND attempts < 5 AND expires_at > now();

-- name: DeleteExpiredVerificationCodes :execrows
DELETE FROM verification_code
WHERE expires_at <= now() OR used_at IS NOT NULL OR attempts >= 5;

-- name: CreateWorkspace :one
INSERT INTO workspace (
    name, slug, description, context, settings, repos,
    issue_prefix, issue_counter, attribution_fail_closed
)
VALUES (
    sqlc.arg(name), sqlc.arg(slug), sqlc.narg(description), '', '{}'::jsonb, '[]'::jsonb,
    sqlc.arg(issue_prefix), 0, false
)
RETURNING *;

-- name: GetWorkspace :one
SELECT * FROM workspace WHERE id = sqlc.arg(id);

-- name: GetWorkspaceBySlug :one
SELECT * FROM workspace WHERE slug = sqlc.arg(slug);

-- name: ListWorkspacesForUser :many
SELECT w.*
FROM workspace w
JOIN member m ON m.workspace_id = w.id
WHERE m.user_id = sqlc.arg(user_id)
ORDER BY w.created_at, w.id;

-- name: UpdateWorkspace :one
UPDATE workspace
SET name = CASE WHEN sqlc.arg(set_name)::boolean THEN sqlc.arg(name) ELSE name END,
    description = CASE WHEN sqlc.arg(set_description)::boolean THEN sqlc.narg(description)::text ELSE description END,
    context = CASE WHEN sqlc.arg(set_context)::boolean THEN sqlc.arg(context) ELSE context END,
    settings = CASE WHEN sqlc.arg(set_settings)::boolean THEN sqlc.arg(settings)::jsonb ELSE settings END,
    repos = CASE WHEN sqlc.arg(set_repos)::boolean THEN sqlc.arg(repos)::jsonb ELSE repos END,
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: LockWorkspaceIssueCounter :one
UPDATE workspace
SET issue_counter = issue_counter + 1,
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING issue_counter, issue_prefix;

-- name: DeleteWorkspace :execrows
DELETE FROM workspace WHERE id = sqlc.arg(id);

-- name: CreateMember :one
INSERT INTO member (workspace_id, user_id, role)
VALUES (sqlc.arg(workspace_id), sqlc.arg(user_id), sqlc.arg(role))
RETURNING *;

-- name: GetMember :one
SELECT *
FROM member
WHERE workspace_id = sqlc.arg(workspace_id) AND user_id = sqlc.arg(user_id);

-- name: GetMemberByID :one
SELECT * FROM member
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: ListWorkspaceMembers :many
SELECT m.id, m.workspace_id, m.user_id, m.role, m.created_at,
       u.name AS user_name, u.email AS user_email
FROM member m
JOIN "user" u ON u.id = m.user_id
WHERE m.workspace_id = sqlc.arg(workspace_id)
ORDER BY m.created_at, m.id;

-- name: DeleteMembersByWorkspace :execrows
DELETE FROM member WHERE workspace_id = sqlc.arg(workspace_id);

-- name: CreatePersonalAccessToken :one
INSERT INTO personal_access_token (user_id, name, token_hash, token_prefix, expires_at)
VALUES (sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(token_hash), sqlc.arg(token_prefix), sqlc.narg(expires_at))
RETURNING *;

-- name: GetPersonalAccessTokenByHash :one
SELECT *
FROM personal_access_token
WHERE token_hash = sqlc.arg(token_hash)
  AND revoked = false
  AND (expires_at IS NULL OR expires_at > now());

-- name: ListPersonalAccessTokens :many
SELECT * FROM personal_access_token
WHERE user_id = sqlc.arg(user_id)
ORDER BY created_at DESC, id DESC;

-- name: RenewPersonalAccessToken :one
UPDATE personal_access_token
SET token_hash = sqlc.arg(token_hash),
    token_prefix = sqlc.arg(token_prefix),
    expires_at = sqlc.narg(expires_at),
    revoked = false
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND revoked = false
RETURNING *;

-- name: RevokePersonalAccessToken :execrows
UPDATE personal_access_token
SET revoked = true
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND revoked = false;

-- name: TouchPersonalAccessToken :execrows
UPDATE personal_access_token SET last_used_at = now() WHERE id = sqlc.arg(id);

-- name: CreateDaemonToken :one
INSERT INTO daemon_token (token_hash, workspace_id, daemon_id, expires_at, user_id, name, token_prefix)
VALUES (
    sqlc.arg(token_hash), sqlc.arg(workspace_id), sqlc.arg(daemon_id), sqlc.arg(expires_at),
    sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(token_prefix)
)
ON CONFLICT (workspace_id, daemon_id) DO UPDATE
SET token_hash = EXCLUDED.token_hash,
    expires_at = EXCLUDED.expires_at,
    user_id = EXCLUDED.user_id,
    name = EXCLUDED.name,
    token_prefix = EXCLUDED.token_prefix,
    created_at = now()
RETURNING *;

-- name: GetDaemonTokenByHash :one
SELECT * FROM daemon_token
WHERE token_hash = sqlc.arg(token_hash) AND expires_at > now();

-- name: GetDaemonTokenByHashForUpdate :one
SELECT * FROM daemon_token
WHERE token_hash = sqlc.arg(token_hash) AND expires_at > now()
FOR UPDATE;

-- name: ListDaemonTokensByWorkspace :many
SELECT * FROM daemon_token
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY created_at DESC, id DESC;

-- name: DeleteDaemonTokenByID :one
DELETE FROM daemon_token
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
RETURNING *;

-- name: DeleteDaemonTokensByWorkspace :execrows
DELETE FROM daemon_token WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteExpiredDaemonTokens :execrows
DELETE FROM daemon_token WHERE expires_at <= now();

-- name: CreateTaskToken :one
INSERT INTO task_token (token_hash, task_id, agent_id, workspace_id, user_id, tool_bundle_id, expires_at)
VALUES (
    sqlc.arg(token_hash), sqlc.arg(task_id), sqlc.arg(agent_id),
    sqlc.arg(workspace_id), sqlc.arg(user_id), sqlc.narg(tool_bundle_id), sqlc.arg(expires_at)
)
RETURNING *;

-- name: GetTaskTokenByHash :one
SELECT * FROM task_token
WHERE token_hash = sqlc.arg(token_hash) AND expires_at > now();

-- name: DeleteTaskTokensByTask :execrows
DELETE FROM task_token WHERE task_id = sqlc.arg(task_id);

-- name: DeleteTaskTokensByWorkspace :execrows
DELETE FROM task_token WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteExpiredTaskTokens :execrows
DELETE FROM task_token WHERE expires_at <= now();
