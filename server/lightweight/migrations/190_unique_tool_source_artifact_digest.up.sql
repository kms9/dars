CREATE UNIQUE INDEX CONCURRENTLY lw_190_unique_tool_source_artifact_digest ON tool_source_artifact (workspace_id, source_id, sha256);
