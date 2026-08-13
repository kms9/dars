CREATE INDEX CONCURRENTLY lw_153_access_daemon_token_expiry ON daemon_token (workspace_id, daemon_id, expires_at);
