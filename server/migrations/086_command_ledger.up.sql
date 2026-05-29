-- Migration: 086_command_ledger
-- Create command_ledger table for immutable audit trail of command executions

CREATE TABLE command_ledger (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    command_run_id UUID NOT NULL REFERENCES command_run(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    template_id UUID REFERENCES command_template(id) ON DELETE SET NULL,
    runtime_id UUID NOT NULL REFERENCES agent_runtime(id) ON DELETE CASCADE,
    command TEXT NOT NULL,
    arguments TEXT[] DEFAULT '{}',
    working_directory TEXT NOT NULL,
    initiator_type TEXT NOT NULL CHECK (initiator_type IN ('member', 'agent')),
    initiator_id UUID NOT NULL,
    status TEXT NOT NULL,
    exit_code INT,
    stdout_hash TEXT,
    stderr_hash TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    duration_ms INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_command_ledger_command_run ON command_ledger(command_run_id);
CREATE INDEX idx_command_ledger_workspace ON command_ledger(workspace_id);
CREATE INDEX idx_command_ledger_created ON command_ledger(created_at DESC);
