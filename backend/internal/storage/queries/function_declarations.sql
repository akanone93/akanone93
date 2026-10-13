-- name: InsertFunctionDeclaration :one
INSERT INTO function_declarations (build_id, organization_id, declaration)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetFunctionDeclaration :one
SELECT * FROM function_declarations WHERE build_id = $1;
