DROP INDEX IF EXISTS idx_preview_registry_workspace_active;

ALTER TABLE preview_registry
    DROP COLUMN IF EXISTS retired_by_id,
    DROP COLUMN IF EXISTS retired_by_type,
    DROP COLUMN IF EXISTS retired_at;
