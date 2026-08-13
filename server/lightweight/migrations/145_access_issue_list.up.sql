CREATE INDEX CONCURRENTLY lw_145_access_issue_list ON issue (workspace_id, updated_at DESC, id DESC);
