-- name: InsertFunctionDeclaration :one
INSERT INTO function_declarations (build_id, organization_id, declaration)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetFunctionDeclaration :one
SELECT * FROM function_declarations WHERE build_id = $1;

-- name: ListFunctionBuildLinks :many
-- The source and the declaration of each build, for the versions made from
-- them. A build without a declaration yields a NULL declaration.
SELECT b.id AS build_id, b.source_id, d.declaration
  FROM function_builds b
  LEFT JOIN function_declarations d ON d.build_id = b.id
 WHERE b.id = ANY(sqlc.arg(build_ids)::uuid[]);
