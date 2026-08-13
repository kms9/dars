CREATE TABLE squad (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    instructions text NOT NULL DEFAULT '',
    leader_id uuid NOT NULL,
    creator_id uuid NOT NULL,
    avatar_url text,
    archived_at timestamptz,
    archived_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE squad_member (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    squad_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    role text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE issue (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    title text NOT NULL,
    description text NOT NULL DEFAULT '',
    status text NOT NULL,
    assignee_type text NOT NULL,
    assignee_id uuid NOT NULL,
    creator_type text NOT NULL,
    creator_id uuid NOT NULL,
    acceptance_criteria jsonb NOT NULL DEFAULT '[]'::jsonb,
    context_refs jsonb NOT NULL DEFAULT '[]'::jsonb,
    number integer NOT NULL,
    idempotency_key text,
    request_hash text,
    first_executed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT issue_status_check CHECK (status IN ('backlog', 'todo', 'in_progress', 'in_review', 'done', 'blocked', 'cancelled')),
    CONSTRAINT issue_assignee_type_check CHECK (assignee_type IN ('agent', 'squad')),
    CONSTRAINT issue_creator_type_check CHECK (creator_type IN ('member', 'agent')),
    CONSTRAINT issue_number_check CHECK (number >= 0),
    CONSTRAINT issue_idempotency_pair_check CHECK ((idempotency_key IS NULL) = (request_hash IS NULL))
);

CREATE TABLE comment (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    issue_id uuid NOT NULL,
    author_type text NOT NULL,
    author_id uuid,
    content text NOT NULL,
    type text NOT NULL DEFAULT 'message',
    source_task_id uuid,
    idempotency_key text,
    request_hash text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT comment_author_type_check CHECK (author_type IN ('member', 'agent', 'system')),
    CONSTRAINT comment_type_check CHECK (type = 'message'),
    CONSTRAINT comment_idempotency_pair_check CHECK ((idempotency_key IS NULL) = (request_hash IS NULL))
);

CREATE TABLE agent_task_queue (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    runtime_id uuid NOT NULL,
    issue_id uuid,
    chat_session_id uuid,
    squad_id uuid,
    is_leader_task boolean NOT NULL DEFAULT false,
    status text NOT NULL,
    priority integer NOT NULL DEFAULT 0,
    attempt integer NOT NULL DEFAULT 1,
    max_attempts integer NOT NULL DEFAULT 2,
    parent_task_id uuid,
    session_id text,
    work_dir text,
    context jsonb NOT NULL DEFAULT '{}'::jsonb,
    result jsonb,
    error text,
    failure_reason text,
    trigger_summary text,
    force_fresh_session boolean NOT NULL DEFAULT false,
    handoff_note text,
    wait_reason text,
    prepare_lease_expires_at timestamptz,
    fire_at timestamptz,
    runtime_mcp_overlay jsonb NOT NULL DEFAULT '{}'::jsonb,
    initiator_user_id uuid,
    originator_user_id uuid,
    accountable_user_id uuid,
    originator_source text,
    trigger_comment_id uuid,
    coalesced_comment_ids uuid[] NOT NULL DEFAULT '{}'::uuid[],
    delivered_comment_ids uuid[] NOT NULL DEFAULT '{}'::uuid[],
    chat_input_task_id uuid,
    chat_finalize_deferred_at timestamptz,
    escalation_for_task_id uuid,
    delegated_from_task_id uuid,
    retry_of_task_id uuid,
    rerun_of_task_id uuid,
    trigger_evidence_kind text,
    trigger_evidence_ref_id uuid,
    session_rollout_missing boolean NOT NULL DEFAULT false,
    retired_session_id text,
    dispatched_at timestamptz,
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT agent_task_status_check CHECK (status IN ('deferred', 'queued', 'dispatched', 'waiting_local_directory', 'running', 'completed', 'failed', 'cancelled')),
    CONSTRAINT agent_task_root_xor_check CHECK ((issue_id IS NULL) <> (chat_session_id IS NULL)),
    CONSTRAINT agent_task_attempt_check CHECK (attempt >= 1 AND max_attempts >= attempt),
    CONSTRAINT agent_task_issue_only_fields_check CHECK (issue_id IS NOT NULL OR (squad_id IS NULL AND is_leader_task = false AND trigger_comment_id IS NULL)),
    CONSTRAINT agent_task_completed_at_check CHECK ((status IN ('completed', 'failed', 'cancelled')) = (completed_at IS NOT NULL))
);

CREATE TABLE task_message (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    task_id uuid NOT NULL,
    seq integer NOT NULL,
    type text NOT NULL,
    tool text,
    content text,
    input jsonb,
    output text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT task_message_seq_check CHECK (seq >= 0)
);

CREATE TABLE task_usage (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    task_id uuid NOT NULL,
    provider text NOT NULL,
    model text NOT NULL,
    input_tokens bigint NOT NULL DEFAULT 0,
    output_tokens bigint NOT NULL DEFAULT 0,
    cache_read_tokens bigint NOT NULL DEFAULT 0,
    cache_write_tokens bigint NOT NULL DEFAULT 0,
    cost_usd_ticks bigint,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT task_usage_non_negative_check CHECK (
        input_tokens >= 0 AND output_tokens >= 0 AND cache_read_tokens >= 0 AND cache_write_tokens >= 0
        AND (cost_usd_ticks IS NULL OR cost_usd_ticks >= 0)
    )
);

CREATE TABLE activity_log (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    issue_id uuid,
    actor_type text NOT NULL,
    actor_id uuid,
    action text NOT NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT activity_actor_type_check CHECK (actor_type IN ('member', 'agent', 'system'))
);
