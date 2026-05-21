-- name: CreateCommandLedgerEntry :one
INSERT INTO command_ledger (
    command_run_id, workspace_id, template_id, runtime_id,
    command, arguments, working_directory,
    initiator_type, initiator_id,
    status, exit_code,
    stdout_hash, stderr_hash,
    started_at, finished_at, duration_ms
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7,
    $8, $9,
    $10, $11,
    $12, $13,
    $14, $15, $16
)
RETURNING *;

-- name: GetCommandLedgerEntry :one
SELECT * FROM command_ledger WHERE id = $1;

-- name: ListCommandLedgerByRun :many
SELECT * FROM command_ledger
WHERE command_run_id = $1
ORDER BY created_at ASC;

-- name: ListCommandLedgerByWorkspace :many
SELECT * FROM command_ledger
WHERE workspace_id = $1
ORDER BY created_at DESC
LIMIT 50;