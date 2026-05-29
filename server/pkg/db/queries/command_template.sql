-- name: GetCommandTemplate :one
SELECT * FROM command_template WHERE id = $1;

-- name: GetCommandTemplateByName :one
SELECT * FROM command_template
WHERE (workspace_id = $1 OR workspace_id IS NULL) AND name = $2 AND is_builtin = true
ORDER BY CASE WHEN workspace_id = $1 THEN 0 ELSE 1 END
LIMIT 1;

-- name: ListCommandTemplates :many
SELECT * FROM command_template
WHERE workspace_id = $1 OR (workspace_id IS NULL AND is_builtin = true)
ORDER BY category ASC, name ASC;
