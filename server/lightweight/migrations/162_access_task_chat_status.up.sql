CREATE INDEX CONCURRENTLY lw_162_access_task_chat_status ON agent_task_queue (chat_session_id, status);
