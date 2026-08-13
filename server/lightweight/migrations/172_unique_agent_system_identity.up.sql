CREATE UNIQUE INDEX CONCURRENTLY lw_172_unique_agent_system_identity ON agent (workspace_id, owner_id, runtime_id, system_key) WHERE system_key IS NOT NULL;
