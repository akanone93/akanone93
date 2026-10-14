-- name: InsertFunctionLogs :exec
INSERT INTO function_logs
    (organization_id, app_id, environment_id, function, version, invocation_id, level, message, trace_id, logged_at)
SELECT sqlc.arg(organization_id)::uuid, sqlc.arg(app_id)::uuid, sqlc.arg(environment_id)::uuid, sqlc.arg(function)::text,
       sqlc.arg(version)::integer, sqlc.arg(invocation_id)::uuid, (sqlc.arg(levels)::text[])[n],
       (sqlc.arg(messages)::text[])[n], sqlc.arg(trace_id)::text, (sqlc.arg(times)::timestamptz[])[n]
  FROM generate_series(1, cardinality(sqlc.arg(levels)::text[])) AS n
 ORDER BY n;

-- name: ListFunctionLogs :many
SELECT * FROM function_logs
 WHERE app_id = sqlc.arg(app_id)
   AND environment_id = sqlc.arg(environment_id)
   AND function = sqlc.arg(function)
   AND (sqlc.arg(version)::integer = 0 OR version = sqlc.arg(version)::integer)
   AND logged_at >= sqlc.arg(since)
   AND (logged_at, id) < (sqlc.arg(before_time)::timestamptz, sqlc.arg(before_id)::bigint)
 ORDER BY logged_at DESC, id DESC
 LIMIT sqlc.arg(page_size);

-- name: PurgeFunctionLogs :execrows
DELETE FROM function_logs WHERE logged_at < $1;

-- name: InsertFunctionInvocation :exec
INSERT INTO function_invocations
    (organization_id, app_id, environment_id, function, version, result, duration_micros, memory_peak_bytes, replayed, finished_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: CountFunctionInvocations :many
SELECT result, replayed, count(*)::bigint AS invocations
  FROM function_invocations
 WHERE app_id = sqlc.arg(app_id)
   AND environment_id = sqlc.arg(environment_id)
   AND function = sqlc.arg(function)
   AND finished_at >= sqlc.arg(since)
 GROUP BY result, replayed;

-- name: FunctionLatency :one
SELECT count(*)::bigint AS runs,
       coalesce(percentile_disc(0.50) WITHIN GROUP (ORDER BY duration_micros), 0)::bigint AS p50,
       coalesce(percentile_disc(0.95) WITHIN GROUP (ORDER BY duration_micros), 0)::bigint AS p95,
       coalesce(percentile_disc(0.99) WITHIN GROUP (ORDER BY duration_micros), 0)::bigint AS p99,
       coalesce(max(memory_peak_bytes), 0)::bigint AS memory_peak
  FROM function_invocations
 WHERE app_id = sqlc.arg(app_id)
   AND environment_id = sqlc.arg(environment_id)
   AND function = sqlc.arg(function)
   AND finished_at >= sqlc.arg(since)
   AND NOT replayed;

-- name: PurgeFunctionInvocations :execrows
DELETE FROM function_invocations WHERE finished_at < $1;
