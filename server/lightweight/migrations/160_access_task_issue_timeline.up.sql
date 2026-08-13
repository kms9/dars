CREATE INDEX CONCURRENTLY lw_160_access_task_issue_timeline ON agent_task_queue (issue_id, created_at, id);
