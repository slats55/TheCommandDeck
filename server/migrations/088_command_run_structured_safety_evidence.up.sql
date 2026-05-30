-- Migration: 088_command_run_structured_safety_evidence
-- Adds structured evidence fields for command-run truncation and cancellation.

ALTER TABLE command_run
    ADD COLUMN stdout_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN stderr_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN cancellation_requested_at TIMESTAMPTZ,
    ADD COLUMN cancellation_requested_by_type TEXT
        CHECK (cancellation_requested_by_type IN ('member', 'agent')),
    ADD COLUMN cancellation_requested_by_id UUID;
