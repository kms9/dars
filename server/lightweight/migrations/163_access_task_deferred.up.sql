CREATE INDEX CONCURRENTLY lw_163_access_task_deferred ON agent_task_queue (status, fire_at) WHERE status = 'deferred';
