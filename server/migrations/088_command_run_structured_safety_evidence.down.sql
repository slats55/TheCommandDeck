-- Migration: 088_command_run_structured_safety_evidence

ALTER TABLE command_run
    DROP COLUMN IF EXISTS cancellation_requested_by_id,
    DROP COLUMN IF EXISTS cancellation_requested_by_type,
    DROP COLUMN IF EXISTS cancellation_requested_at,
    DROP COLUMN IF EXISTS stderr_truncated,
    DROP COLUMN IF EXISTS stdout_truncated;
