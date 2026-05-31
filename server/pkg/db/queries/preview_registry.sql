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
    runtime_id = COALESCE(EXCLUDED.runtime_id, preview_registry.runtime_id),
    name = EXCLUDED.name,
    port = EXCLUDED.port,
    status = EXCLUDED.status,
    last_checked_at = EXCLUDED.last_checked_at,
    last_success_at = COALESCE(EXCLUDED.last_success_at, preview_registry.last_success_at),
    command_run_id = COALESCE(EXCLUDED.command_run_id, preview_registry.command_run_id),
    retired_at = NULL,
    retired_by_type = NULL,
    retired_by_id = NULL,
    updated_at = now()
RETURNING *;

-- name: RetirePreviewRegistryRecord :one
UPDATE preview_registry
SET
    retired_at = COALESCE(retired_at, now()),
    retired_by_type = COALESCE(retired_by_type, @retired_by_type),
    retired_by_id = COALESCE(retired_by_id, @retired_by_id),
    updated_at = now()
WHERE id = @id
  AND workspace_id = @workspace_id
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
  AND pr.retired_at IS NULL
ORDER BY pr.created_at ASC;
