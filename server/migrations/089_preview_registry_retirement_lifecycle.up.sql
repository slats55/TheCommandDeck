-- Migration: 089_preview_registry_retirement_lifecycle
-- Add non-destructive retirement metadata for preview lifecycle control.

ALTER TABLE preview_registry
    ADD COLUMN retired_at TIMESTAMPTZ,
    ADD COLUMN retired_by_type TEXT,
    ADD COLUMN retired_by_id UUID;

CREATE INDEX idx_preview_registry_workspace_active
    ON preview_registry(workspace_id, updated_at DESC)
    WHERE retired_at IS NULL;
