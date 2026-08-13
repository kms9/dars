ALTER TABLE agent
    ADD COLUMN kind text NOT NULL DEFAULT 'user',
    ADD COLUMN system_key text,
    ADD CONSTRAINT agent_kind_check CHECK (kind IN ('user', 'system'));
