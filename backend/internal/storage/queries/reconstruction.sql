-- name: InsertDeviceInstall :exec
INSERT INTO device_installs (id, organization_id, app_id, environment_id, device_id, channel_key, release_sequence, bundles,
                             device_class, locale, theme, text_scale_percent, user_ref, reported_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: LatestDeviceInstall :one
SELECT * FROM device_installs WHERE device_id = $1 ORDER BY reported_at DESC, id DESC LIMIT 1;

-- name: DeviceInstallAt :one
-- The state a device had reported at a moment: its newest row at or before it.
SELECT * FROM device_installs
 WHERE device_id = $1 AND reported_at <= sqlc.arg(moment)::timestamptz
 ORDER BY reported_at DESC, id DESC LIMIT 1;

-- name: UserInstallAt :one
-- The device a pseudonymous user was last reported on at or before a moment.
SELECT * FROM device_installs
 WHERE app_id = $1 AND user_ref = sqlc.arg(user_ref)::text AND reported_at <= sqlc.arg(moment)::timestamptz
 ORDER BY reported_at DESC, id DESC LIMIT 1;

-- name: LastRouteAt :one
-- The page a device last reported viewing at or before a moment, within the
-- same release; the events kept are the ones recorded under consent.
SELECT route FROM analytics_events
 WHERE app_id = $1 AND environment_id = $2 AND device_id = $3 AND release_sequence = $4
   AND route <> '' AND time <= sqlc.arg(moment)::timestamptz AND time > sqlc.arg(since)::timestamptz
 ORDER BY time DESC LIMIT 1;

-- name: PurgeDeviceInstalls :execrows
-- History older than the retention, except each device's newest row at or
-- before the cut-off: it is the state the device was in when the retained
-- window began.
DELETE FROM device_installs d
 WHERE d.organization_id = $1 AND d.reported_at < sqlc.arg(before)::timestamptz
   AND EXISTS (SELECT 1 FROM device_installs n
                WHERE n.device_id = d.device_id AND n.reported_at < sqlc.arg(before)::timestamptz
                  AND (n.reported_at, n.id) > (d.reported_at, d.id));

-- name: PurgeReconstructions :execrows
DELETE FROM reconstructions WHERE organization_id = $1 AND created_at < sqlc.arg(before)::timestamptz;

-- name: GetControlDocumentAt :one
-- The control document in force at a moment: the newest signed version
-- issued by then.
SELECT * FROM control_documents
 WHERE channel_id = $1 AND signed IS NOT NULL AND issued_at <= sqlc.arg(moment)::timestamptz
 ORDER BY version DESC LIMIT 1;

-- name: OldestSignedControlDocument :one
SELECT * FROM control_documents WHERE channel_id = $1 AND signed IS NOT NULL ORDER BY version LIMIT 1;

-- name: GetVersionByBundle :one
-- A retained version of an app's plugin (the empty key is the app bundle) by its bundle hash.
SELECT * FROM plugin_versions WHERE app_id = $1 AND plugin_key = $2 AND bundle_sha256 = $3 ORDER BY version DESC LIMIT 1;

-- name: InsertReconstruction :one
INSERT INTO reconstructions (id, organization_id, app_id, environment_id, channel_id, device_id, subject_kind, moment,
                             release_sequence, manifest, created_by_kind, created_by_id, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: GetReconstruction :one
SELECT * FROM reconstructions WHERE id = $1;

-- name: GetReconstructionForUpdate :one
SELECT * FROM reconstructions WHERE id = $1 FOR UPDATE;

-- name: StoreReconstructionSignature :one
UPDATE reconstructions SET signed = $2, signatures = $3, signed_at = $4
 WHERE id = $1 AND signed IS NULL
RETURNING *;

-- name: ListReconstructions :many
SELECT * FROM reconstructions
 WHERE app_id = $1 AND (created_at, id) < (sqlc.arg(before_time)::timestamptz, sqlc.arg(before_id)::uuid)
 ORDER BY created_at DESC, id DESC
 LIMIT sqlc.arg(page_size);
