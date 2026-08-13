CREATE TABLE agent_builder_draft (
    chat_session_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    draft jsonb NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now()
);
