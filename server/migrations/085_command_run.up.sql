-- Migration: 085_command_run
-- Create command_run table for tracking command executions

CREATE TABLE command_run (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    template_id UUID REFERENCES command_template(id) ON DELETE SET NULL,
    runtime_id UUID NOT NULL REFERENCES agent_runtime(id) ON DELETE CASCADE,
    issue_id UUID REFERENCES issue(id) ON DELETE SET NULL,

    command TEXT NOT NULL,
    arguments TEXT[] DEFAULT '{}',
    working_directory TEXT NOT NULL,

    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'completed', 'failed', 'timeout', 'cancelled')),

    exit_code INT,
    stdout TEXT,
    stderr TEXT,
    full_log_path TEXT,

    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    duration_ms INT,

    initiator_type TEXT NOT NULL CHECK (initiator_type IN ('member', 'agent')),
    initiator_id UUID NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_command_run_workspace ON command_run(workspace_id);
CREATE INDEX idx_command_run_runtime ON command_run(runtime_id);
CREATE INDEX idx_command_run_status ON command_run(status);
CREATE INDEX idx_command_run_created ON command_run(created_at DESC);