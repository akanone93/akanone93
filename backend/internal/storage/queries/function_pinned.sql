-- name: GetPinnedFunction :one
-- The pin of a function version by an active release: a release that a
-- channel of the environment currently points at, and that pins the
-- version. A release carries the environment it was created in, so the
-- channel, not the pin, selects the environment: a promoted release pins
-- the same versions there (FN-006, ADR-0063).
SELECT rf.module_sha256, rf.placement
  FROM release_functions rf
  JOIN releases r ON r.id = rf.release_id
  JOIN channels c ON c.environment_id = $2 AND c.release_sequence = r.sequence
 WHERE rf.app_id = $1 AND rf.name = $3 AND rf.version = $4
 LIMIT 1;

-- name: GetEnvironmentVariablesByKey :many
SELECT key, value FROM environment_variables WHERE environment_id = $1 AND key = ANY(sqlc.arg(keys)::text[]);
