CREATE INDEX CONCURRENTLY lw_147_access_issue_assignee ON issue (workspace_id, assignee_type, assignee_id, updated_at DESC, id DESC);
