-- name: CreateCommandWorkflowExecution :one
INSERT INTO command_workflow_execution (
    workspace_id,
    project_id,
    command_run_id,
    title,
    objective,
    status,
    created_by_type,
    created_by_id
) VALUES (
    @workspace_id,
    @project_id,
    @command_run_id,
    @title,
    @objective,
    @status,
    @created_by_type,
    @created_by_id
)
RETURNING *;

-- name: UpdateCommandWorkflowExecutionStatus :one
UPDATE command_workflow_execution
SET
    status = @status,
    updated_at = now()
WHERE id = @id
  AND workspace_id = @workspace_id
RETURNING *;

-- name: GetCommandWorkflowExecution :one
SELECT
    cwe.*,
    cr.status AS command_run_status,
    cr.command AS command_run_command,
    p.title AS project_title
FROM command_workflow_execution cwe
LEFT JOIN command_run cr ON cr.id = cwe.command_run_id
LEFT JOIN project p ON p.id = cwe.project_id
WHERE cwe.id = @id
  AND cwe.workspace_id = @workspace_id;

-- name: ListCommandWorkflowExecutions :many
SELECT
    cwe.*,
    cr.status AS command_run_status,
    cr.command AS command_run_command,
    p.title AS project_title
FROM command_workflow_execution cwe
LEFT JOIN command_run cr ON cr.id = cwe.command_run_id
LEFT JOIN project p ON p.id = cwe.project_id
WHERE cwe.workspace_id = @workspace_id
ORDER BY cwe.updated_at DESC, cwe.created_at DESC;
