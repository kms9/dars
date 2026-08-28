ALTER TABLE daemon_token
    ADD COLUMN user_id uuid,
    ADD COLUMN name text,
    ADD COLUMN token_prefix text;

UPDATE daemon_token
SET name = daemon_id;

UPDATE daemon_token AS token
SET user_id = owner.user_id
FROM (
    SELECT workspace_id, daemon_id, MIN(owner_id::text)::uuid AS user_id
    FROM agent_runtime
    GROUP BY workspace_id, daemon_id
    HAVING COUNT(DISTINCT owner_id) = 1
) AS owner
WHERE token.workspace_id = owner.workspace_id
  AND token.daemon_id = owner.daemon_id;
