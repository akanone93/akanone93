-- name: InsertFunctionSource :one
INSERT INTO function_sources
    (id, organization_id, app_id, environment_id, name, object_key, sha256, size_bytes, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetFunctionSource :one
SELECT * FROM function_sources WHERE id = $1;

-- name: InsertFunctionBuild :one
INSERT INTO function_builds
    (id, organization_id, app_id, environment_id, source_id, name, placement, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetFunctionBuild :one
SELECT * FROM function_builds WHERE id = $1;

-- name: GetFunctionBuildForUpdate :one
SELECT * FROM function_builds WHERE id = $1 FOR UPDATE;

-- name: StartFunctionBuild :one
UPDATE function_builds
   SET state = 'running', started_at = sqlc.arg(started_at)
 WHERE id = $1 AND state IN ('queued', 'running')
RETURNING *;

-- name: FailFunctionBuild :one
UPDATE function_builds
   SET state = 'failed', diagnostics = sqlc.arg(diagnostics), test_results = sqlc.arg(test_results),
       finished_at = sqlc.arg(finished_at)
 WHERE id = $1 AND state IN ('queued', 'running')
RETURNING *;

-- name: SucceedFunctionBuild :one
UPDATE function_builds
   SET state = 'succeeded', placement = sqlc.arg(placement), diagnostics = sqlc.arg(diagnostics),
       test_results = sqlc.arg(test_results), module_key = sqlc.arg(module_key),
       module_sha256 = sqlc.arg(module_sha256), abi_version = sqlc.arg(abi_version),
       functions_manifest = sqlc.arg(functions_manifest), provenance = sqlc.arg(provenance),
       statement = sqlc.arg(statement), signature = sqlc.arg(signature), key_id = sqlc.arg(key_id),
       finished_at = sqlc.arg(finished_at)
 WHERE id = $1 AND state = 'running'
RETURNING *;

-- name: LockFunction :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(scope)::text, 0));

-- name: NextFunctionVersion :one
SELECT (coalesce(max(version), 0) + 1)::int AS version
  FROM function_versions
 WHERE environment_id = $1 AND name = $2;

-- name: InsertFunctionVersion :one
INSERT INTO function_versions
    (organization_id, app_id, environment_id, name, version, build_id, placement, module_key,
     module_sha256, abi_version, input_schema, output_schema, provenance, statement, signature,
     key_id, tests_passed, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
RETURNING *;

-- name: GetFunctionVersion :one
SELECT * FROM function_versions WHERE environment_id = $1 AND name = $2 AND version = $3;

-- name: GetLatestFunctionVersion :one
SELECT * FROM function_versions WHERE environment_id = $1 AND name = $2
 ORDER BY version DESC LIMIT 1;

-- name: ListLatestFunctionVersions :many
SELECT DISTINCT ON (name) *
  FROM function_versions
 WHERE environment_id = $1 AND name > sqlc.arg(after_name)::text
 ORDER BY name, version DESC
 LIMIT sqlc.arg(page_size);

-- name: ListFunctionVersions :many
SELECT * FROM function_versions
 WHERE environment_id = $1 AND name = $2
   AND (sqlc.arg(before_version)::int = 0 OR version < sqlc.arg(before_version)::int)
 ORDER BY version DESC
 LIMIT sqlc.arg(page_size);

-- name: UpsertFunctionAlias :one
INSERT INTO function_aliases
    (organization_id, app_id, environment_id, name, alias, version, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (environment_id, name, alias) DO UPDATE
   SET version = EXCLUDED.version, updated_by = EXCLUDED.updated_by, updated_at = now()
RETURNING *;

-- name: GetFunctionAlias :one
SELECT * FROM function_aliases WHERE environment_id = $1 AND name = $2 AND alias = $3;

-- name: ListFunctionAliases :many
SELECT * FROM function_aliases WHERE environment_id = $1 AND name = $2 ORDER BY alias;

-- name: GetFunctionKV :one
SELECT value FROM function_kv
 WHERE environment_id = $1 AND function_name = $2 AND key = $3;

-- name: GetFunctionKVForUpdate :one
SELECT octet_length(key) + octet_length(value) AS size_bytes FROM function_kv
 WHERE environment_id = $1 AND function_name = $2 AND key = $3 FOR UPDATE;

-- name: FunctionKVUsage :one
SELECT count(*)::bigint AS keys,
       coalesce(sum(octet_length(key) + octet_length(value)), 0)::bigint AS total_bytes
  FROM function_kv
 WHERE environment_id = $1 AND function_name = $2;

-- name: UpsertFunctionKV :exec
INSERT INTO function_kv (organization_id, app_id, environment_id, function_name, key, value)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (environment_id, function_name, key) DO UPDATE
   SET value = EXCLUDED.value, updated_at = now();

-- name: DeleteFunctionKV :execrows
DELETE FROM function_kv WHERE environment_id = $1 AND function_name = $2 AND key = $3;
