CREATE INDEX CONCURRENTLY lw_193_access_tool_source_revision_status ON tool_source_revision (workspace_id, source_id, status, revision DESC);
