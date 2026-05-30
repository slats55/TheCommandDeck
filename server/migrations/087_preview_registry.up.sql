-- Migration: 087_preview_registry
-- Persist trusted CommandDeck preview observations.

CREATE TABLE preview_registry (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    runtime_id UUID REFERENCES agent_runtime(id) ON DELETE SET NULL,
    command_run_id UUID REFERENCES command_run(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    preview_url TEXT NOT NULL,
    port INT NOT NULL CHECK (port >= 1 AND port <= 65535),
    source TEXT NOT NULL CHECK (source IN ('self_hosted_stack')),
    status TEXT NOT NULL CHECK (status IN ('healthy', 'unhealthy', 'unavailable', 'unknown')),
    last_checked_at TIMESTAMPTZ,
    last_success_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT preview_registry_preview_url_no_control CHECK (preview_url !~ '[[:cntrl:]]')
);

CREATE UNIQUE INDEX idx_preview_registry_workspace_source_url
    ON preview_registry(workspace_id, source, preview_url);

CREATE INDEX idx_preview_registry_workspace
    ON preview_registry(workspace_id, updated_at DESC);

CREATE INDEX idx_preview_registry_runtime
    ON preview_registry(runtime_id)
    WHERE runtime_id IS NOT NULL;
