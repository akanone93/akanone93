-- name: SummariseExperimentMetric :many
-- One row per variant with the sufficient statistics of a metric over the
-- devices exposed to the experiment: the number of devices and the sums of the
-- outcome y (after the device's first exposure), of the covariate x (the same
-- metric in the days before it), and of y*y, x*x and x*y. A proportion's
-- outcome is 1 for a device that did the event, otherwise 0; a mean's is the
-- sum of the event's numeric property over the device's events.
WITH exposure AS (
    SELECT DISTINCT ON (x.device_id) x.device_id, x.fields->>'variant' AS variant, x.time AS exposed_at
      FROM analytics_events x
     WHERE x.app_id = sqlc.arg(app_id) AND x.environment_id = sqlc.arg(environment_id)
       AND x.name = 'experiment_exposure' AND x.fields->>'experiment' = sqlc.arg(experiment)::text
       AND x.time >= sqlc.arg(since) AND x.time < sqlc.arg(until)
     ORDER BY x.device_id, x.time, x.id
), post AS (
    SELECT m.device_id, count(*) AS events,
           coalesce(sum(CASE WHEN jsonb_typeof(m.fields->sqlc.arg(property)::text) = 'number'
                             THEN (m.fields->>sqlc.arg(property)::text)::float8 END), 0) AS total
      FROM analytics_events m
      JOIN exposure e ON e.device_id = m.device_id
     WHERE m.app_id = sqlc.arg(app_id) AND m.environment_id = sqlc.arg(environment_id)
       AND m.name = sqlc.arg(metric)::text AND m.time >= e.exposed_at AND m.time < sqlc.arg(until)
     GROUP BY m.device_id
), pre AS (
    SELECT m.device_id, count(*) AS events,
           coalesce(sum(CASE WHEN jsonb_typeof(m.fields->sqlc.arg(property)::text) = 'number'
                             THEN (m.fields->>sqlc.arg(property)::text)::float8 END), 0) AS total
      FROM analytics_events m
      JOIN exposure e ON e.device_id = m.device_id
     WHERE m.app_id = sqlc.arg(app_id) AND m.environment_id = sqlc.arg(environment_id)
       AND m.name = sqlc.arg(metric)::text
       AND m.time >= e.exposed_at - make_interval(days => sqlc.arg(pre_days)::int) AND m.time < e.exposed_at
     GROUP BY m.device_id
), scored AS (
    SELECT e.variant,
           CASE WHEN sqlc.arg(kind)::text = 'mean' THEN coalesce(post.total, 0)
                ELSE (coalesce(post.events, 0) > 0)::int::float8 END AS y,
           CASE WHEN sqlc.arg(kind)::text = 'mean' THEN coalesce(pre.total, 0)
                ELSE (coalesce(pre.events, 0) > 0)::int::float8 END AS x
      FROM exposure e
      LEFT JOIN post ON post.device_id = e.device_id
      LEFT JOIN pre ON pre.device_id = e.device_id
)
SELECT variant::text AS variant, count(*)::bigint AS subjects,
       sum(y)::float8 AS sum_y, sum(y * y)::float8 AS sum_yy,
       sum(x)::float8 AS sum_x, sum(x * x)::float8 AS sum_xx, sum(x * y)::float8 AS sum_xy
  FROM scored
 GROUP BY variant
 ORDER BY variant;
