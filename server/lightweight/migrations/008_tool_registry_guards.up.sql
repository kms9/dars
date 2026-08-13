CREATE FUNCTION lw_validate_tool_source_write() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF EXISTS (SELECT 1 FROM tool_source_revision revision WHERE revision.workspace_id = OLD.workspace_id AND revision.source_id = OLD.id) OR
           EXISTS (SELECT 1 FROM tool_definition definition WHERE definition.workspace_id = OLD.workspace_id AND definition.source_id = OLD.id) OR
           EXISTS (SELECT 1 FROM tool_source_artifact artifact WHERE artifact.workspace_id = OLD.workspace_id AND artifact.source_id = OLD.id) OR
           EXISTS (SELECT 1 FROM tool_source_secret secret WHERE secret.workspace_id = OLD.workspace_id AND secret.source_id = OLD.id) OR
           EXISTS (SELECT 1 FROM tool_bundle_item item WHERE item.workspace_id = OLD.workspace_id AND item.source_id = OLD.id) THEN
            RAISE EXCEPTION 'tool source is still retained';
        END IF;
        RETURN OLD;
    END IF;
    IF TG_OP = 'UPDATE' AND (
        NEW.id IS DISTINCT FROM OLD.id OR
        NEW.workspace_id IS DISTINCT FROM OLD.workspace_id OR
        NEW.kind IS DISTINCT FROM OLD.kind OR
        NEW.created_by IS DISTINCT FROM OLD.created_by OR
        NEW.created_at IS DISTINCT FROM OLD.created_at
    ) THEN
        RAISE EXCEPTION 'immutable tool source identity';
    END IF;
    IF NEW.current_revision IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM tool_source_revision revision
        WHERE revision.id = NEW.current_revision
          AND revision.workspace_id = NEW.workspace_id
          AND revision.source_id = NEW.id
          AND revision.status = 'ready'
    ) THEN
        RAISE EXCEPTION 'invalid current tool source revision';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER lw_tool_source_write_guard
BEFORE INSERT OR UPDATE OR DELETE ON tool_source
FOR EACH ROW EXECUTE FUNCTION lw_validate_tool_source_write();

CREATE FUNCTION lw_validate_tool_source_artifact_write() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION 'tool source artifacts are immutable';
    END IF;
    IF TG_OP = 'DELETE' THEN
        IF EXISTS (
            SELECT 1 FROM tool_bundle_item item
            WHERE item.workspace_id = OLD.workspace_id AND item.artifact_id = OLD.id
        ) OR EXISTS (
            SELECT 1 FROM tool_source_revision revision
            WHERE revision.workspace_id = OLD.workspace_id AND revision.artifact_id = OLD.id
        ) THEN
            RAISE EXCEPTION 'tool source artifact is retained by a bundle';
        END IF;
        RETURN OLD;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM tool_source source
        WHERE source.id = NEW.source_id AND source.workspace_id = NEW.workspace_id
    ) THEN
        RAISE EXCEPTION 'invalid tool source artifact scope';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER lw_tool_source_artifact_write_guard
BEFORE INSERT OR UPDATE OR DELETE ON tool_source_artifact
FOR EACH ROW EXECUTE FUNCTION lw_validate_tool_source_artifact_write();

CREATE FUNCTION lw_validate_tool_source_secret_write() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF EXISTS (
            SELECT 1 FROM tool_source_revision revision
            WHERE revision.workspace_id = OLD.workspace_id AND revision.secret_id = OLD.id
        ) THEN
            RAISE EXCEPTION 'tool source secret is retained by a revision';
        END IF;
        RETURN OLD;
    END IF;
    IF TG_OP = 'UPDATE' AND (
        NEW.id IS DISTINCT FROM OLD.id OR
        NEW.workspace_id IS DISTINCT FROM OLD.workspace_id OR
        NEW.source_id IS DISTINCT FROM OLD.source_id OR
        NEW.created_at IS DISTINCT FROM OLD.created_at
    ) THEN
        RAISE EXCEPTION 'immutable tool source secret identity';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM tool_source source
        WHERE source.id = NEW.source_id AND source.workspace_id = NEW.workspace_id
    ) THEN
        RAISE EXCEPTION 'invalid tool source secret scope';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER lw_tool_source_secret_write_guard
BEFORE INSERT OR UPDATE OR DELETE ON tool_source_secret
FOR EACH ROW EXECUTE FUNCTION lw_validate_tool_source_secret_write();

CREATE FUNCTION lw_validate_tool_source_revision_write() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF EXISTS (
            SELECT 1 FROM tool_bundle_item item
            WHERE item.workspace_id = OLD.workspace_id AND item.source_revision_id = OLD.id
        ) OR EXISTS (
            SELECT 1 FROM tool_definition definition
            WHERE definition.workspace_id = OLD.workspace_id AND definition.source_revision_id = OLD.id
        ) OR EXISTS (
            SELECT 1 FROM tool_source source
            WHERE source.workspace_id = OLD.workspace_id AND source.current_revision = OLD.id
        ) THEN
            RAISE EXCEPTION 'tool source revision is retained by a bundle';
        END IF;
        RETURN OLD;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM tool_source source
        WHERE source.id = NEW.source_id AND source.workspace_id = NEW.workspace_id
    ) THEN
        RAISE EXCEPTION 'invalid tool source revision scope';
    END IF;
    IF NEW.artifact_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM tool_source_artifact artifact
        WHERE artifact.id = NEW.artifact_id
          AND artifact.workspace_id = NEW.workspace_id
          AND artifact.source_id = NEW.source_id
    ) THEN
        RAISE EXCEPTION 'invalid tool source revision artifact';
    END IF;
    IF NEW.secret_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM tool_source_secret secret
        WHERE secret.id = NEW.secret_id
          AND secret.workspace_id = NEW.workspace_id
          AND secret.source_id = NEW.source_id
    ) THEN
        RAISE EXCEPTION 'invalid tool source revision secret';
    END IF;
    IF TG_OP = 'UPDATE' THEN
        IF NEW.id IS DISTINCT FROM OLD.id OR
           NEW.workspace_id IS DISTINCT FROM OLD.workspace_id OR
           NEW.source_id IS DISTINCT FROM OLD.source_id OR
           NEW.revision IS DISTINCT FROM OLD.revision OR
           NEW.endpoint IS DISTINCT FROM OLD.endpoint OR
           NEW.transport_config IS DISTINCT FROM OLD.transport_config OR
           NEW.artifact_id IS DISTINCT FROM OLD.artifact_id OR
           NEW.secret_id IS DISTINCT FROM OLD.secret_id OR
           NEW.created_by IS DISTINCT FROM OLD.created_by OR
           NEW.created_at IS DISTINCT FROM OLD.created_at THEN
            RAISE EXCEPTION 'immutable tool source revision content';
        END IF;
        IF NOT (
            (OLD.status = 'validating' AND NEW.status IN ('validating', 'ready', 'failed')) OR
            (OLD.status = 'ready' AND NEW.status IN ('ready', 'retired')) OR
            (OLD.status = NEW.status)
        ) THEN
            RAISE EXCEPTION 'invalid tool source revision transition';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER lw_tool_source_revision_write_guard
BEFORE INSERT OR UPDATE OR DELETE ON tool_source_revision
FOR EACH ROW EXECUTE FUNCTION lw_validate_tool_source_revision_write();

CREATE FUNCTION lw_validate_tool_definition_write() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF EXISTS (
            SELECT 1 FROM tool_bundle_item item
            WHERE item.workspace_id = OLD.workspace_id AND item.tool_definition_id = OLD.id
        ) THEN
            RAISE EXCEPTION 'tool definition is retained by a bundle';
        END IF;
        RETURN OLD;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM tool_source_revision revision
        WHERE revision.id = NEW.source_revision_id
          AND revision.workspace_id = NEW.workspace_id
          AND revision.source_id = NEW.source_id
    ) THEN
        RAISE EXCEPTION 'invalid tool definition scope';
    END IF;
    IF TG_OP = 'UPDATE' AND (
        NEW.id IS DISTINCT FROM OLD.id OR
        NEW.workspace_id IS DISTINCT FROM OLD.workspace_id OR
        NEW.source_id IS DISTINCT FROM OLD.source_id OR
        NEW.source_revision_id IS DISTINCT FROM OLD.source_revision_id OR
        NEW.public_name IS DISTINCT FROM OLD.public_name OR
        NEW.upstream_name IS DISTINCT FROM OLD.upstream_name OR
        NEW.description IS DISTINCT FROM OLD.description OR
        NEW.input_schema IS DISTINCT FROM OLD.input_schema OR
        NEW.output_schema IS DISTINCT FROM OLD.output_schema OR
        NEW.operation_metadata IS DISTINCT FROM OLD.operation_metadata OR
        NEW.created_at IS DISTINCT FROM OLD.created_at
    ) THEN
        RAISE EXCEPTION 'immutable tool definition content';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER lw_tool_definition_write_guard
BEFORE INSERT OR UPDATE OR DELETE ON tool_definition
FOR EACH ROW EXECUTE FUNCTION lw_validate_tool_definition_write();

CREATE FUNCTION lw_validate_tool_bundle_write() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.status <> 'revoked' OR
           EXISTS (SELECT 1 FROM agent_tool_bundle_head head WHERE head.workspace_id = OLD.workspace_id AND head.bundle_id = OLD.id) OR
           EXISTS (SELECT 1 FROM agent_task_queue task WHERE task.workspace_id = OLD.workspace_id AND task.tool_bundle_id = OLD.id) OR
           EXISTS (SELECT 1 FROM task_token token WHERE token.workspace_id = OLD.workspace_id AND token.tool_bundle_id = OLD.id) OR
           EXISTS (SELECT 1 FROM tool_bundle_item item WHERE item.workspace_id = OLD.workspace_id AND item.bundle_id = OLD.id) THEN
            RAISE EXCEPTION 'tool bundle is still retained';
        END IF;
        RETURN OLD;
    END IF;
    IF TG_OP = 'UPDATE' THEN
        IF NEW.id IS DISTINCT FROM OLD.id OR
           NEW.workspace_id IS DISTINCT FROM OLD.workspace_id OR
           NEW.manifest_hash IS DISTINCT FROM OLD.manifest_hash OR
           NEW.created_by IS DISTINCT FROM OLD.created_by OR
           NEW.created_at IS DISTINCT FROM OLD.created_at THEN
            RAISE EXCEPTION 'immutable tool bundle manifest';
        END IF;
        IF NOT (OLD.status = 'active' AND NEW.status = 'revoked') AND NEW.status IS DISTINCT FROM OLD.status THEN
            RAISE EXCEPTION 'invalid tool bundle transition';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER lw_tool_bundle_write_guard
BEFORE UPDATE OR DELETE ON tool_bundle
FOR EACH ROW EXECUTE FUNCTION lw_validate_tool_bundle_write();

CREATE FUNCTION lw_validate_tool_bundle_item_write() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION 'tool bundle items are immutable';
    END IF;
    IF TG_OP = 'DELETE' THEN
        IF EXISTS (
            SELECT 1 FROM tool_bundle bundle
            WHERE bundle.id = OLD.bundle_id
              AND bundle.workspace_id = OLD.workspace_id
              AND (
                  bundle.status <> 'revoked' OR
                  EXISTS (SELECT 1 FROM agent_tool_bundle_head head WHERE head.workspace_id = OLD.workspace_id AND head.bundle_id = OLD.bundle_id) OR
                  EXISTS (SELECT 1 FROM agent_task_queue task WHERE task.workspace_id = OLD.workspace_id AND task.tool_bundle_id = OLD.bundle_id) OR
                  EXISTS (SELECT 1 FROM task_token token WHERE token.workspace_id = OLD.workspace_id AND token.tool_bundle_id = OLD.bundle_id)
              )
        ) THEN
            RAISE EXCEPTION 'tool bundle item is still retained';
        END IF;
        RETURN OLD;
    END IF;
    IF NOT EXISTS (
        SELECT 1
        FROM tool_bundle bundle
        JOIN tool_source source ON source.id = NEW.source_id AND source.workspace_id = NEW.workspace_id
        JOIN tool_source_revision revision ON revision.id = NEW.source_revision_id
            AND revision.workspace_id = NEW.workspace_id
            AND revision.source_id = NEW.source_id
        JOIN tool_definition definition ON definition.id = NEW.tool_definition_id
            AND definition.workspace_id = NEW.workspace_id
            AND definition.source_id = NEW.source_id
            AND definition.source_revision_id = NEW.source_revision_id
        WHERE bundle.id = NEW.bundle_id
          AND bundle.workspace_id = NEW.workspace_id
          AND bundle.status = 'active'
          AND source.enabled = true
          AND revision.status = 'ready'
          AND definition.enabled = true
    ) THEN
        RAISE EXCEPTION 'invalid tool bundle item scope';
    END IF;
    IF NEW.artifact_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM tool_source_artifact artifact
        WHERE artifact.id = NEW.artifact_id
          AND artifact.workspace_id = NEW.workspace_id
          AND artifact.source_id = NEW.source_id
    ) THEN
        RAISE EXCEPTION 'invalid tool bundle item artifact';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER lw_tool_bundle_item_write_guard
BEFORE INSERT OR UPDATE OR DELETE ON tool_bundle_item
FOR EACH ROW EXECUTE FUNCTION lw_validate_tool_bundle_item_write();

CREATE FUNCTION lw_validate_agent_tool_bundle_head_write() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND (
        NEW.id IS DISTINCT FROM OLD.id OR
        NEW.workspace_id IS DISTINCT FROM OLD.workspace_id OR
        NEW.agent_id IS DISTINCT FROM OLD.agent_id
    ) THEN
        RAISE EXCEPTION 'immutable agent tool bundle head identity';
    END IF;
    IF NOT EXISTS (
        SELECT 1
        FROM agent
        JOIN tool_bundle bundle ON bundle.id = NEW.bundle_id AND bundle.workspace_id = NEW.workspace_id
        WHERE agent.id = NEW.agent_id
          AND agent.workspace_id = NEW.workspace_id
          AND agent.archived_at IS NULL
          AND bundle.status = 'active'
    ) THEN
        RAISE EXCEPTION 'invalid agent tool bundle head scope';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER lw_agent_tool_bundle_head_write_guard
BEFORE INSERT OR UPDATE ON agent_tool_bundle_head
FOR EACH ROW EXECUTE FUNCTION lw_validate_agent_tool_bundle_head_write();

CREATE FUNCTION lw_validate_agent_task_bundle_pin() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.tool_bundle_id IS NOT NULL AND NEW.tool_bundle_id IS DISTINCT FROM OLD.tool_bundle_id THEN
        RAISE EXCEPTION 'task tool bundle pin is write-once';
    END IF;
    IF NEW.tool_bundle_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM tool_bundle bundle
        WHERE bundle.id = NEW.tool_bundle_id
          AND bundle.workspace_id = NEW.workspace_id
          AND bundle.status = 'active'
    ) THEN
        RAISE EXCEPTION 'invalid task tool bundle scope';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER lw_agent_task_bundle_pin_guard
BEFORE INSERT OR UPDATE OF tool_bundle_id ON agent_task_queue
FOR EACH ROW EXECUTE FUNCTION lw_validate_agent_task_bundle_pin();

CREATE FUNCTION lw_validate_task_token_bundle_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM agent_task_queue task
        WHERE task.id = NEW.task_id
          AND task.workspace_id = NEW.workspace_id
          AND task.agent_id = NEW.agent_id
          AND task.tool_bundle_id IS NOT DISTINCT FROM NEW.tool_bundle_id
    ) THEN
        RAISE EXCEPTION 'invalid task token bundle scope';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER lw_task_token_bundle_scope_guard
BEFORE INSERT OR UPDATE ON task_token
FOR EACH ROW EXECUTE FUNCTION lw_validate_task_token_bundle_scope();
