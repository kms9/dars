CREATE INDEX CONCURRENTLY lw_198_access_agent_task_bundle ON agent_task_queue (workspace_id, tool_bundle_id) WHERE tool_bundle_id IS NOT NULL;
