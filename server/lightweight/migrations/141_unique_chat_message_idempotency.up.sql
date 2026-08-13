CREATE UNIQUE INDEX CONCURRENTLY lw_141_unique_chat_message_idempotency ON chat_message (chat_session_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
