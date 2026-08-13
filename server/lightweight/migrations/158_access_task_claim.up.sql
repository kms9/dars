CREATE INDEX CONCURRENTLY lw_158_access_task_claim ON agent_task_queue (runtime_id, status, priority DESC, created_at);
