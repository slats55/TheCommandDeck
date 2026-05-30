-- name: UpsertPreviewRegistryRecord :one
INSERT INTO preview_registry (
    workspace_id,
    runtime_id,
    command_run_id,
    name,
    preview_url,
    port,
    source,
    status,
    last_checked_at,
    last_success_at
) VALUES (
    @workspace_id,
    @runtime_id,
    @command_run_id,
    @name,
    @preview_url,
    @port,
    @source,
    @status,
    @last_checked_at,
    @last_success_at
)
ON CONFLICT (workspace_id, source, preview_url)
DO UPDATE SET
    runtime_id = EXCLUDED.runtime_id,
    name = EXCLUDED.name,
    port = EXCLUDED.port,
    status = EXCLUDED.status,
    last_checked_at = EXCLUDED.last_checked_at,
    last_success_at = COALESCE(EXCLUDED.last_success_at, preview_registry.last_success_at),
    command_run_id = COALESCE(EXCLUDED.command_run_id, preview_registry.command_run_id),
    updated_at = now()
RETURNING *;

-- name: ListPreviewRegistryRecords :many
SELECT
    pr.*,
    ar.name AS runtime_name,
    ar.status AS runtime_status,
    ar.daemon_id AS runtime_daemon_id
FROM preview_registry pr
LEFT JOIN agent_runtime ar ON ar.id = pr.runtime_id
WHERE pr.workspace_id = @workspace_id
ORDER BY pr.created_at ASC;
