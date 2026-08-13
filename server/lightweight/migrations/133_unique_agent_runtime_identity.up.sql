CREATE UNIQUE INDEX CONCURRENTLY lw_133_unique_agent_runtime_identity ON agent_runtime (workspace_id, daemon_id, provider, profile_id) NULLS NOT DISTINCT;
