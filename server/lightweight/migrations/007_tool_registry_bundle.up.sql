CREATE TABLE tool_source (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    name text NOT NULL,
    kind text NOT NULL,
    enabled boolean NOT NULL DEFAULT false,
    current_revision uuid,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT tool_source_name_check CHECK (name = lower(name) AND name ~ '^[a-z0-9][a-z0-9_-]{0,62}$'),
    CONSTRAINT tool_source_kind_check CHECK (kind IN ('server_local', 'remote_mcp', 'grpc', 'openapi')),
    CONSTRAINT tool_source_enabled_revision_check CHECK (enabled = false OR current_revision IS NOT NULL)
);

CREATE TABLE tool_source_artifact (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    source_id uuid NOT NULL,
    sha256 text NOT NULL,
    media_type text NOT NULL,
    size_bytes bigint NOT NULL,
    content bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT tool_source_artifact_sha256_check CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT tool_source_artifact_size_check CHECK (size_bytes >= 0 AND size_bytes = octet_length(content)),
    CONSTRAINT tool_source_artifact_media_type_check CHECK (length(media_type) BETWEEN 1 AND 255)
);

CREATE TABLE tool_source_secret (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    source_id uuid NOT NULL,
    envelope jsonb NOT NULL,
    key_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT tool_source_secret_envelope_check CHECK (jsonb_typeof(envelope) = 'object'),
    CONSTRAINT tool_source_secret_key_id_check CHECK (key_id ~ '^[A-Za-z0-9._-]{1,64}$')
);

CREATE TABLE tool_source_revision (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    source_id uuid NOT NULL,
    revision integer NOT NULL,
    status text NOT NULL DEFAULT 'validating',
    endpoint text,
    transport_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    artifact_id uuid,
    secret_id uuid,
    validation_code text,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    CONSTRAINT tool_source_revision_number_check CHECK (revision >= 1),
    CONSTRAINT tool_source_revision_status_check CHECK (status IN ('validating', 'ready', 'failed', 'retired')),
    CONSTRAINT tool_source_revision_transport_check CHECK (jsonb_typeof(transport_config) = 'object'),
    CONSTRAINT tool_source_revision_publish_check CHECK ((status = 'ready') = (published_at IS NOT NULL))
);

CREATE TABLE tool_definition (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    source_id uuid NOT NULL,
    source_revision_id uuid NOT NULL,
    public_name text NOT NULL,
    upstream_name text NOT NULL,
    description text NOT NULL DEFAULT '',
    input_schema jsonb NOT NULL,
    output_schema jsonb,
    operation_metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT tool_definition_public_name_check CHECK (public_name ~ '^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$'),
    CONSTRAINT tool_definition_upstream_name_check CHECK (length(upstream_name) BETWEEN 1 AND 255),
    CONSTRAINT tool_definition_input_schema_check CHECK (jsonb_typeof(input_schema) = 'object'),
    CONSTRAINT tool_definition_output_schema_check CHECK (output_schema IS NULL OR jsonb_typeof(output_schema) = 'object'),
    CONSTRAINT tool_definition_operation_metadata_check CHECK (jsonb_typeof(operation_metadata) = 'object')
);

CREATE TABLE tool_bundle (
    id text NOT NULL,
    workspace_id uuid NOT NULL,
    manifest_hash text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    CONSTRAINT tool_bundle_id_check CHECK (id ~ '^tb_[A-Za-z0-9_-]{5,77}$'),
    CONSTRAINT tool_bundle_manifest_hash_check CHECK (manifest_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT tool_bundle_status_check CHECK (status IN ('active', 'revoked')),
    CONSTRAINT tool_bundle_revoke_check CHECK ((status = 'revoked') = (revoked_at IS NOT NULL))
);

CREATE TABLE tool_bundle_item (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    bundle_id text NOT NULL,
    ordinal integer NOT NULL,
    public_name text NOT NULL,
    source_id uuid NOT NULL,
    source_revision_id uuid NOT NULL,
    tool_definition_id uuid NOT NULL,
    artifact_id uuid,
    definition_snapshot jsonb NOT NULL,
    invocation_plan jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT tool_bundle_item_ordinal_check CHECK (ordinal >= 0),
    CONSTRAINT tool_bundle_item_public_name_check CHECK (public_name ~ '^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$'),
    CONSTRAINT tool_bundle_item_definition_check CHECK (jsonb_typeof(definition_snapshot) = 'object'),
    CONSTRAINT tool_bundle_item_invocation_check CHECK (jsonb_typeof(invocation_plan) = 'object')
);

CREATE TABLE agent_tool_bundle_head (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    bundle_id text NOT NULL,
    updated_by uuid NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE agent_task_queue ADD COLUMN tool_bundle_id text;
ALTER TABLE task_token ADD COLUMN tool_bundle_id text;
