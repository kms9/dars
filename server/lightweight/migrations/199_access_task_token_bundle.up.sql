CREATE INDEX CONCURRENTLY lw_199_access_task_token_bundle ON task_token (workspace_id, task_id, tool_bundle_id) WHERE tool_bundle_id IS NOT NULL;
