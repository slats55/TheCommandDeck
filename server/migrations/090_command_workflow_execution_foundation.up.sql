-- Migration: 090_command_workflow_execution_foundation
-- Introduce workspace-scoped workflow execution records for CommandDeck.

CREATE TABLE command_workflow_execution (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    project_id UUID REFERENCES project(id) ON DELETE SET NULL,
    command_run_id UUID REFERENCES command_run(id) ON DELETE SET NULL,
    title TEXT NOT NULL CHECK (length(trim(title)) > 0 AND length(title) <= 200),
    objective TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'planned'
        CHECK (status IN ('planned', 'running', 'needs_review', 'completed', 'failed', 'cancelled')),
    created_by_type TEXT NOT NULL CHECK (created_by_type IN ('member', 'agent')),
    created_by_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_command_workflow_execution_workspace_updated
    ON command_workflow_execution(workspace_id, updated_at DESC, created_at DESC);

CREATE INDEX idx_command_workflow_execution_command_run
    ON command_workflow_execution(command_run_id)
    WHERE command_run_id IS NOT NULL;
