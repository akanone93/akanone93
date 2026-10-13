-- name: GetPinnedFunction :one
-- The pin of a function version by an active release: a release that a
-- channel of the environment currently points at (FN-006, ADR-0063).
SELECT rf.module_sha256, rf.placement
  FROM release_functions rf
  JOIN releases r ON r.id = rf.release_id
  JOIN channels c ON c.environment_id = r.environment_id AND c.release_sequence = r.sequence
 WHERE rf.app_id = $1 AND rf.environment_id = $2 AND rf.name = $3 AND rf.version = $4
 LIMIT 1;

-- name: GetFunctionDeclaration :one
SELECT declaration FROM function_declarations WHERE build_id = $1;
