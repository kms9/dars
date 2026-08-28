ALTER TABLE daemon_token
    DROP COLUMN token_prefix,
    DROP COLUMN name,
    DROP COLUMN user_id;
