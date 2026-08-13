CREATE UNIQUE INDEX CONCURRENTLY lw_139_unique_issue_idempotency ON issue (workspace_id, creator_type, creator_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
