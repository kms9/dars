ALTER TABLE agent
    DROP CONSTRAINT IF EXISTS agent_kind_check,
    DROP COLUMN IF EXISTS system_key,
    DROP COLUMN IF EXISTS kind;
