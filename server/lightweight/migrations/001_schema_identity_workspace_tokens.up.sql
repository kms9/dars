CREATE TABLE schema_metadata (
    id smallint NOT NULL DEFAULT 1,
    edition text NOT NULL DEFAULT 'dars_lightweight',
    schema_version integer NOT NULL,
    installed_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT schema_metadata_identity_check CHECK (id = 1 AND edition = 'dars_lightweight')
);

INSERT INTO schema_metadata (id, edition, schema_version, installed_at, updated_at)
VALUES (1, 'dars_lightweight', 1, now(), now());

CREATE TABLE "user" (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text NOT NULL,
    email text NOT NULL,
    language text NOT NULL DEFAULT 'en',
    timezone text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE verification_code (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    email text NOT NULL,
    code_hash text NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    attempts integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT verification_code_attempts_check CHECK (attempts >= 0)
);

CREATE TABLE workspace (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    context text NOT NULL DEFAULT '',
    settings jsonb NOT NULL DEFAULT '{}'::jsonb,
    repos jsonb NOT NULL DEFAULT '[]'::jsonb,
    issue_prefix text NOT NULL,
    issue_counter integer NOT NULL DEFAULT 0,
    attribution_fail_closed boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT workspace_issue_counter_check CHECK (issue_counter >= 0)
);

CREATE TABLE member (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT member_role_check CHECK (role IN ('owner', 'admin', 'member'))
);

CREATE TABLE personal_access_token (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL,
    name text NOT NULL,
    token_hash text NOT NULL,
    token_prefix text NOT NULL,
    expires_at timestamptz,
    last_used_at timestamptz,
    revoked boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE daemon_token (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    token_hash text NOT NULL,
    workspace_id uuid NOT NULL,
    daemon_id text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE task_token (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    token_hash text NOT NULL,
    task_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    user_id uuid NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
