-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Data subject requests: the export and the erasure of what is held about a
-- device or a pseudonymous user (SEC-162). Each export query returns one
-- JSON document per row, at most row_limit of them, and names the columns it
-- exports: key material, secret hashes and the sealed push token are never
-- among them. Each erase query deletes one store's rows of one subject.

-- name: DsrExportDevice :many
SELECT jsonb_build_object(
           'id', id, 'appId', app_id, 'environmentId', environment_id, 'platform', platform,
           'osVersion', os_version, 'runtimeVersion', runtime_version, 'hostBuild', host_build,
           'assuranceLevel', assurance_level, 'installedSequence', installed_sequence,
           'registeredAt', registered_at, 'lastSeenAt', last_seen_at, 'deviceClass', device_class,
           'locale', locale, 'keyStorage', key_storage, 'attestationProvider', attestation_provider,
           'attestedAt', attested_at, 'revokedAt', revoked_at, 'revokedReason', revoked_reason,
           'raspDetectedAt', rasp_detected_at, 'raspKinds', rasp_kinds)::jsonb AS document
  FROM devices
 WHERE organization_id = $1 AND id = $2
 LIMIT sqlc.arg(row_limit);

-- name: DsrExportDeviceBundles :many
SELECT jsonb_build_object('pluginKey', plugin_key, 'bundleSha256', encode(bundle_sha256, 'hex'))::jsonb AS document
  FROM device_bundles
 WHERE organization_id = $1 AND device_id = $2
 ORDER BY plugin_key
 LIMIT sqlc.arg(row_limit);

-- name: DsrExportDevSessionDevices :many
SELECT jsonb_build_object('devSession', session_id, 'name', name, 'platform', platform, 'runtimeVersion', runtime_version, 'pairedAt', paired_at)::jsonb AS document
  FROM dev_session_devices
 WHERE organization_id = $1 AND device_id = (sqlc.arg(device_id)::uuid)::text
 ORDER BY paired_at, id
 LIMIT sqlc.arg(row_limit);

-- name: DsrExportErrorGroupDevices :many
SELECT jsonb_build_object('errorGroup', group_id)::jsonb AS document
  FROM error_group_devices
 WHERE organization_id = $1 AND device_id = $2
 ORDER BY group_id
 LIMIT sqlc.arg(row_limit);

-- name: DsrExportDeviceTokens :many
SELECT jsonb_build_object('id', id, 'expiresAt', expires_at)::jsonb AS document
  FROM device_tokens
 WHERE organization_id = $1 AND device_id = $2
 ORDER BY id
 LIMIT sqlc.arg(row_limit);

-- name: DsrExportDeviceSCAKeys :many
SELECT jsonb_build_object('jkt', jkt, 'keyStorage', key_storage, 'attestationProvider', attestation_provider,
                          'enrolledAt', enrolled_at)::jsonb AS document
  FROM device_sca_keys
 WHERE organization_id = $1 AND device_id = $2
 LIMIT sqlc.arg(row_limit);

-- name: DsrExportDevicePushTokens :many
SELECT jsonb_build_object('appId', app_id, 'environmentId', environment_id, 'provider', provider,
                          'updatedAt', updated_at, 'tokenHeld', true)::jsonb AS document
  FROM device_push_tokens
 WHERE organization_id = $1 AND device_id = $2
 LIMIT sqlc.arg(row_limit);

-- name: DsrExportTelemetryEvents :many
SELECT jsonb_build_object('id', id, 'appId', app_id, 'environmentId', environment_id, 'name', name,
                          'time', time, 'releaseSequence', release_sequence, 'pluginKey', plugin_key,
                          'route', route, 'fields', fields, 'receivedAt', received_at)::jsonb AS document
  FROM telemetry_events
 WHERE organization_id = $1 AND device_id = $2
 ORDER BY time, id
 LIMIT sqlc.arg(row_limit);

-- name: DsrExportAnalyticsEvents :many
SELECT jsonb_build_object('id', id, 'appId', app_id, 'environmentId', environment_id, 'name', name,
                          'time', time, 'releaseSequence', release_sequence, 'pluginKey', plugin_key,
                          'route', route, 'fields', fields, 'receivedAt', received_at,
                          'deviceClass', device_class, 'locale', locale, 'category', category)::jsonb AS document
  FROM analytics_events
 WHERE organization_id = $1 AND device_id = $2
 ORDER BY time, id
 LIMIT sqlc.arg(row_limit);

-- name: DsrExportNotifications :many
SELECT jsonb_build_object('id', id, 'eventId', event_id, 'eventType', event_type, 'appId', app_id,
                          'title', title, 'body', body, 'link', link, 'createdAt', created_at,
                          'readAt', read_at)::jsonb AS document
  FROM notifications
 WHERE organization_id = $1 AND user_id = $2
 ORDER BY created_at, id
 LIMIT sqlc.arg(row_limit);

-- name: DsrExportNotificationSubscriptions :many
SELECT jsonb_build_object('id', id, 'eventType', event_type, 'channelId', channel_id)::jsonb AS document
  FROM notification_subscriptions
 WHERE organization_id = $1 AND subject_kind = 'user' AND subject_id = $2
 ORDER BY id
 LIMIT sqlc.arg(row_limit);

-- name: DsrEraseTelemetryEvents :execrows
DELETE FROM telemetry_events WHERE organization_id = $1 AND device_id = $2;

-- name: DsrEraseAnalyticsEvents :execrows
DELETE FROM analytics_events WHERE organization_id = $1 AND device_id = $2;

-- name: DsrErasePushTokens :execrows
DELETE FROM device_push_tokens WHERE organization_id = $1 AND device_id = $2;

-- name: DsrEraseSCAKeys :execrows
DELETE FROM device_sca_keys WHERE organization_id = $1 AND device_id = $2;

-- name: DsrEraseDeviceTokens :execrows
DELETE FROM device_tokens WHERE organization_id = $1 AND device_id = $2;

-- name: DsrEraseDevSessionDevices :execrows
DELETE FROM dev_session_devices WHERE organization_id = $1 AND device_id = (sqlc.arg(device_id)::uuid)::text;

-- name: DsrEraseErrorGroupDevices :execrows
DELETE FROM error_group_devices WHERE organization_id = $1 AND device_id = $2;

-- name: DsrEraseDeviceBundles :execrows
DELETE FROM device_bundles WHERE organization_id = $1 AND device_id = $2;

-- The device record is last: with its credentials gone the device cannot
-- authenticate and has to register again.
-- name: DsrEraseDevice :execrows
DELETE FROM devices WHERE organization_id = $1 AND id = $2;

-- name: DsrEraseNotifications :execrows
DELETE FROM notifications WHERE organization_id = $1 AND user_id = $2;

-- name: DsrEraseNotificationSubscriptions :execrows
DELETE FROM notification_subscriptions WHERE organization_id = $1 AND subject_kind = 'user' AND subject_id = $2;

-- name: InsertDsrTombstone :exec
INSERT INTO dsr_tombstones (organization_id, request_id, subject_kind, subject_hash, store, row_count, erased_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: CountDsrTombstones :one
SELECT count(*) FROM dsr_tombstones WHERE organization_id = $1 AND subject_hash = $2;

-- name: InsertDsrRequest :exec
INSERT INTO dsr_requests (id, organization_id, action, subject_kind, subject_hash, actor, row_count, archive_bytes, requested_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetDsrRequest :one
SELECT * FROM dsr_requests WHERE id = $1;

-- name: ListDsrRequests :many
SELECT * FROM dsr_requests
 WHERE organization_id = $1
   AND (requested_at, id) < (sqlc.arg(before_time)::timestamptz, sqlc.arg(before_id)::uuid)
 ORDER BY requested_at DESC, id DESC
 LIMIT sqlc.arg(page_size);

-- name: ListDsrRequestTombstones :many
SELECT store, row_count FROM dsr_tombstones WHERE request_id = $1 ORDER BY id;

-- name: DsrExportDeviceInstalls :many
SELECT jsonb_build_object('reportedAt', reported_at, 'channel', channel_key, 'releaseSequence', release_sequence,
                          'bundles', bundles, 'deviceClass', device_class, 'locale', locale, 'theme', theme,
                          'textScalePercent', text_scale_percent, 'userRef', user_ref)::jsonb AS document
  FROM device_installs
 WHERE organization_id = $1 AND device_id = $2
 ORDER BY reported_at, id
 LIMIT sqlc.arg(row_limit);

-- name: DsrEraseDeviceInstalls :execrows
DELETE FROM device_installs WHERE organization_id = $1 AND device_id = $2;

-- name: DsrExportReconstructions :many
SELECT jsonb_build_object('id', id, 'moment', moment, 'releaseSequence', release_sequence, 'createdAt', created_at,
                          'manifest', manifest)::jsonb AS document
  FROM reconstructions
 WHERE organization_id = $1 AND device_id = $2
 ORDER BY created_at, id
 LIMIT sqlc.arg(row_limit);

-- name: DsrEraseReconstructions :execrows
DELETE FROM reconstructions WHERE organization_id = $1 AND device_id = $2;
