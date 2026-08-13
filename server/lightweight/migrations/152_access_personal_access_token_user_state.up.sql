CREATE INDEX CONCURRENTLY lw_152_access_personal_access_token_user_state ON personal_access_token (user_id, revoked, expires_at);
