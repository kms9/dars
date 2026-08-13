CREATE TABLE chat_session (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    creator_id uuid NOT NULL,
    runtime_id uuid NOT NULL,
    title text NOT NULL,
    session_id text,
    work_dir text,
    status text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chat_session_status_check CHECK (status IN ('active', 'archived'))
);

CREATE TABLE chat_message (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    chat_session_id uuid NOT NULL,
    role text NOT NULL,
    content text NOT NULL,
    task_id uuid,
    failure_reason text,
    elapsed_ms bigint,
    message_kind text NOT NULL DEFAULT 'message',
    idempotency_key text,
    request_hash text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chat_message_role_check CHECK (role IN ('user', 'assistant', 'system')),
    CONSTRAINT chat_message_kind_check CHECK (message_kind IN ('message', 'no_response')),
    CONSTRAINT chat_message_elapsed_check CHECK (elapsed_ms IS NULL OR elapsed_ms >= 0),
    CONSTRAINT chat_message_idempotency_pair_check CHECK ((idempotency_key IS NULL) = (request_hash IS NULL))
);

CREATE TABLE chat_draft_restore (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    chat_session_id uuid NOT NULL,
    task_id uuid NOT NULL,
    content text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
