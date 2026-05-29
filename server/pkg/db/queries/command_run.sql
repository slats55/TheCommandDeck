-- name: CreateCommandRun :one
INSERT INTO command_run (
    workspace_id, template_id, runtime_id, issue_id,
    command, arguments, working_directory, status,
    initiator_type, initiator_id
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8,
    $9, $10
)
RETURNING *;

-- name: GetCommandRun :one
SELECT * FROM command_run WHERE id = $1;

-- name: ListCommandRuns :many
SELECT * FROM command_run
WHERE workspace_id = $1
ORDER BY created_at DESC
LIMIT 50;

-- name: UpdateCommandRunResult :one
UPDATE command_run
SET
    status = $2,
    exit_code = $3,
    stdout = $4,
    stderr = $5,
    finished_at = $6,
    duration_ms = $7,
    started_at = $8
WHERE id = $1
RETURNING *;