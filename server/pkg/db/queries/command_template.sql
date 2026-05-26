-- name: GetCommandTemplate :one
SELECT * FROM command_template WHERE id = $1;

-- name: GetCommandTemplateByName :one
SELECT * FROM command_template
WHERE (workspace_id = $1 OR workspace_id = '00000000-0000-0000-0000-000000000000') AND name = $2 AND is_builtin = true;

-- name: ListCommandTemplates :many
SELECT * FROM command_template
WHERE workspace_id = $1
ORDER BY category ASC, name ASC;