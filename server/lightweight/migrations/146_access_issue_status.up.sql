CREATE INDEX CONCURRENTLY lw_146_access_issue_status ON issue (workspace_id, status, updated_at DESC, id DESC);
