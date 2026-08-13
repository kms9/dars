CREATE INDEX CONCURRENTLY lw_164_access_task_prepare_lease ON agent_task_queue (status, prepare_lease_expires_at) WHERE status IN ('dispatched','waiting_local_directory');
