CREATE TABLE runtime_profile (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    display_name text NOT NULL,
    protocol_family text NOT NULL,
    command_name text NOT NULL,
    description text,
    fixed_args jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_by uuid NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE agent_runtime (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    daemon_id text NOT NULL,
    name text NOT NULL,
    runtime_mode text NOT NULL DEFAULT 'local',
    provider text NOT NULL,
    status text NOT NULL DEFAULT 'offline',
    device_info text NOT NULL DEFAULT '',
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    last_seen_at timestamptz,
    owner_id uuid NOT NULL,
    profile_id uuid,
    custom_name text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT agent_runtime_mode_check CHECK (runtime_mode = 'local'),
    CONSTRAINT agent_runtime_status_check CHECK (status IN ('online', 'offline'))
);

CREATE TABLE agent (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    runtime_id uuid,
    owner_id uuid NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    instructions text NOT NULL DEFAULT '',
    avatar_url text,
    runtime_mode text NOT NULL DEFAULT 'local',
    runtime_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'offline',
    max_concurrent_tasks integer NOT NULL DEFAULT 1,
    custom_env jsonb NOT NULL DEFAULT '{}'::jsonb,
    custom_args jsonb NOT NULL DEFAULT '[]'::jsonb,
    mcp_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    model text,
    thinking_level text,
    service_tier text,
    permission_mode text NOT NULL,
    disabled_runtime_skills jsonb NOT NULL DEFAULT '[]'::jsonb,
    archived_at timestamptz,
    archived_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT agent_runtime_mode_check CHECK (runtime_mode = 'local'),
    CONSTRAINT agent_status_check CHECK (status IN ('idle', 'working', 'blocked', 'error', 'offline')),
    CONSTRAINT agent_max_concurrent_tasks_check CHECK (max_concurrent_tasks >= 1),
    CONSTRAINT agent_permission_mode_check CHECK (permission_mode IN ('private', 'public_to'))
);

CREATE TABLE skill (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    content text NOT NULL DEFAULT '',
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE skill_file (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    skill_id uuid NOT NULL,
    path text NOT NULL,
    content text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE agent_skill (
    agent_id uuid NOT NULL,
    skill_id uuid NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE agent_invocation_target (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    agent_id uuid NOT NULL,
    target_type text NOT NULL,
    target_id uuid NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT agent_invocation_target_type_check CHECK (target_type IN ('workspace', 'member'))
);
