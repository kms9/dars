CREATE UNIQUE INDEX CONCURRENTLY lw_140_unique_comment_idempotency ON comment (workspace_id, issue_id, author_type, author_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
