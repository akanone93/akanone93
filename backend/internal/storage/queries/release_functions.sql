-- name: InsertReleaseFunction :exec
INSERT INTO release_functions (release_id, organization_id, app_id, environment_id, name, version, module_sha256, placement)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: CopyReleaseFunctions :exec
INSERT INTO release_functions (release_id, organization_id, app_id, environment_id, name, version, module_sha256, placement)
SELECT sqlc.arg(release_id)::uuid, organization_id, app_id, environment_id, name, version, module_sha256, placement
  FROM release_functions
 WHERE release_id = sqlc.arg(from_release_id)::uuid;

-- name: ListReleaseFunctions :many
SELECT * FROM release_functions
 WHERE release_id = ANY(sqlc.arg(release_ids)::uuid[])
 ORDER BY release_id, name;
