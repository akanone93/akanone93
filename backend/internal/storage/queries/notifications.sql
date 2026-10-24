-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertNotificationChannel :one
INSERT INTO notification_channels (id, organization_id, name, kind, settings, ciphertext, wrapped_key, key_id, hint, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetNotificationChannel :one
SELECT * FROM notification_channels WHERE id = $1;

-- name: ListNotificationChannels :many
SELECT * FROM notification_channels ORDER BY name;

-- name: DeleteNotificationChannel :execrows
DELETE FROM notification_channels WHERE id = $1;

-- name: InsertNotificationSubscription :execrows
INSERT INTO notification_subscriptions (id, organization_id, subject_kind, subject_id, event_type, channel_id, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT DO NOTHING;

-- name: DeleteNotificationSubscription :execrows
DELETE FROM notification_subscriptions WHERE id = $1;

-- name: GetNotificationSubscription :one
SELECT * FROM notification_subscriptions WHERE id = $1;

-- name: ListNotificationSubscriptions :many
SELECT * FROM notification_subscriptions
 WHERE (sqlc.narg('subject_kind')::text IS NULL OR subject_kind = sqlc.narg('subject_kind'))
   AND (sqlc.narg('subject_id')::uuid IS NULL OR subject_id = sqlc.narg('subject_id'))
 ORDER BY event_type, subject_kind, subject_id, id;

-- name: ListSubscriptionsForEvent :many
SELECT * FROM notification_subscriptions WHERE event_type = $1 ORDER BY id;

-- name: IsOrganizationMember :one
SELECT EXISTS (SELECT 1 FROM memberships WHERE organization_id = $1 AND user_id = $2) AS member;

-- name: TeamExists :one
SELECT EXISTS (SELECT 1 FROM teams WHERE id = $1) AS found;

-- The enabled users of a team, with the address a mail goes to.
-- name: ListTeamRecipients :many
SELECT u.id, u.email FROM memberships m JOIN users u ON u.id = m.user_id
 WHERE m.team_id = $1 AND u.disabled_at IS NULL
 ORDER BY u.id;

-- name: GetUserRecipient :one
SELECT id, email FROM users WHERE id = $1 AND disabled_at IS NULL;

-- name: InsertNotification :exec
INSERT INTO notifications (id, organization_id, user_id, event_id, event_type, app_id, title, body, link, created_at, params, group_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- One row per group, the newest, with the number of rows the group holds
-- among those listed. A row without a group key (written before grouping)
-- is a group of its own.
-- name: ListNotificationGroups :many
SELECT sqlc.embed(n), g.group_count::integer AS group_count
  FROM (SELECT DISTINCT ON (COALESCE(NULLIF(s.group_key, ''), s.id::text))
               s.id, count(*) OVER (PARTITION BY COALESCE(NULLIF(s.group_key, ''), s.id::text)) AS group_count
          FROM notifications s
         WHERE s.user_id = sqlc.arg('user_id') AND (NOT sqlc.arg('unread_only')::boolean OR s.read_at IS NULL)
         ORDER BY COALESCE(NULLIF(s.group_key, ''), s.id::text), s.created_at DESC, s.id DESC) g
  JOIN notifications n ON n.id = g.id
 ORDER BY n.created_at DESC, n.id DESC
 LIMIT sqlc.arg('page_size');

-- Deletes up to batch_size notifications created before the cutoff, oldest
-- first, for the retention sweep.
-- name: PurgeNotifications :execrows
DELETE FROM notifications WHERE id IN (
    SELECT s.id FROM notifications s
     WHERE s.created_at < sqlc.arg('cutoff')
     ORDER BY s.created_at, s.id
     LIMIT sqlc.arg('batch_size'));

-- name: CountUnreadNotifications :one
SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkNotificationsRead :execrows
UPDATE notifications SET read_at = now()
 WHERE user_id = $1 AND read_at IS NULL AND (cardinality(sqlc.arg('ids')::uuid[]) = 0 OR id = ANY(sqlc.arg('ids')::uuid[]));

-- Locks the hour's row so concurrent publishers count correctly.
-- name: LockNotificationHour :one
INSERT INTO notification_hours (organization_id, hour) VALUES ($1, $2)
ON CONFLICT (organization_id, hour) DO UPDATE SET n = notification_hours.n
RETURNING n, dropped;

-- name: AddNotificationHour :exec
UPDATE notification_hours SET n = n + $3, dropped = dropped + $4 WHERE organization_id = $1 AND hour = $2;

-- name: InsertWebhookEndpoint :one
INSERT INTO webhook_endpoints (id, organization_id, name, url, event_types, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetWebhookEndpoint :one
SELECT * FROM webhook_endpoints WHERE id = $1;

-- name: ListWebhookEndpoints :many
SELECT * FROM webhook_endpoints ORDER BY name;

-- name: CountWebhookEndpoints :one
SELECT count(*) FROM webhook_endpoints;

-- name: DeleteWebhookEndpoint :execrows
DELETE FROM webhook_endpoints WHERE id = $1;

-- name: ListWebhookEndpointsForEvent :many
SELECT * FROM webhook_endpoints
 WHERE enabled AND (cardinality(event_types) = 0 OR $1::text = ANY(event_types))
 ORDER BY id;

-- name: InsertWebhookSecret :exec
INSERT INTO webhook_secrets (id, organization_id, owner_kind, owner_id, ciphertext, wrapped_key, key_id, hint)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- Ends the validity of the secrets with no expiry yet; the newest one is
-- inserted after, so it is the only one left without.
-- name: ExpireWebhookSecrets :exec
UPDATE webhook_secrets SET expires_at = $3
 WHERE owner_kind = $1 AND owner_id = $2 AND expires_at IS NULL;

-- name: ListActiveWebhookSecrets :many
SELECT * FROM webhook_secrets
 WHERE owner_kind = $1 AND owner_id = $2 AND (expires_at IS NULL OR expires_at > $3)
 ORDER BY created_at DESC, id DESC;

-- name: DeleteWebhookSecrets :exec
DELETE FROM webhook_secrets WHERE owner_kind = $1 AND owner_id = $2;

-- name: PurgeExpiredWebhookSecrets :execrows
DELETE FROM webhook_secrets WHERE expires_at IS NOT NULL AND expires_at <= $1;

-- name: InsertDelivery :exec
INSERT INTO notification_deliveries (id, organization_id, kind, channel_id, endpoint_id, event_id, event_type, recipient, payload, next_attempt_at, expires_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- Claims due deliveries with a lease: next_attempt_at moves past the time
-- the attempt may take, so a crash retries once the lease runs out.
-- name: ClaimDueDeliveries :many
UPDATE notification_deliveries d SET next_attempt_at = $2
 WHERE d.id IN (
    SELECT s.id FROM notification_deliveries s
     WHERE s.state = 'pending' AND s.next_attempt_at <= $1
     ORDER BY s.next_attempt_at, s.id
     LIMIT $3
     FOR UPDATE SKIP LOCKED)
RETURNING d.*;

-- name: MarkDeliveryDelivered :exec
UPDATE notification_deliveries
   SET state = 'delivered', attempts = attempts + 1, last_status = $2, last_error = '', delivered_at = $3
 WHERE id = $1;

-- name: MarkDeliveryRetry :exec
UPDATE notification_deliveries
   SET attempts = attempts + 1, next_attempt_at = $2, last_status = $3, last_error = $4
 WHERE id = $1;

-- name: MarkDeliveryFailed :exec
UPDATE notification_deliveries
   SET state = 'failed', attempts = attempts + 1, last_status = $2, last_error = $3
 WHERE id = $1;

-- name: ListDeliveries :many
SELECT * FROM notification_deliveries
 WHERE (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind'))
   AND (sqlc.narg('endpoint_id')::uuid IS NULL OR endpoint_id = sqlc.narg('endpoint_id'))
   AND (sqlc.narg('channel_id')::uuid IS NULL OR channel_id = sqlc.narg('channel_id'))
   AND (sqlc.narg('state')::text IS NULL OR state = sqlc.narg('state'))
 ORDER BY created_at DESC, id DESC
 LIMIT sqlc.arg('page_size');

-- name: PurgeDeliveries :execrows
DELETE FROM notification_deliveries WHERE state <> 'pending' AND created_at < $1;

-- name: InsertFunctionTrigger :one
INSERT INTO function_triggers (id, organization_id, app_id, environment_id, function_name, alias, expression, timezone, input, next_fire_at, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetFunctionTrigger :one
SELECT * FROM function_triggers WHERE id = $1;

-- name: ListFunctionTriggers :many
SELECT * FROM function_triggers WHERE app_id = $1 ORDER BY function_name, alias, id;

-- name: DeleteFunctionTrigger :execrows
DELETE FROM function_triggers WHERE id = $1;

-- name: ListDueFunctionTriggers :many
SELECT * FROM function_triggers
 WHERE enabled AND next_fire_at <= $1
 ORDER BY next_fire_at, id
 LIMIT $2
 FOR UPDATE SKIP LOCKED;

-- name: AdvanceFunctionTrigger :exec
UPDATE function_triggers SET next_fire_at = $2, last_fire_at = $3 WHERE id = $1;

-- name: InsertTriggerFire :execrows
INSERT INTO function_trigger_fires (trigger_id, fire_at, organization_id) VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: FinishTriggerFire :exec
UPDATE function_trigger_fires SET result = $3 WHERE trigger_id = $1 AND fire_at = $2;

-- name: SetFunctionTriggerResult :exec
UPDATE function_triggers SET last_result = $2 WHERE id = $1;

-- name: PurgeTriggerFires :execrows
DELETE FROM function_trigger_fires WHERE started_at < $1;

-- name: InsertFunctionHook :one
INSERT INTO function_hooks (id, organization_id, app_id, environment_id, function_name, alias, verifier, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetFunctionHook :one
SELECT * FROM function_hooks WHERE id = $1;

-- name: GetFunctionHookByName :one
SELECT * FROM function_hooks WHERE app_id = $1 AND function_name = $2;

-- name: ListFunctionHooks :many
SELECT * FROM function_hooks WHERE app_id = $1 ORDER BY function_name;

-- name: DeleteFunctionHook :execrows
DELETE FROM function_hooks WHERE id = $1;

-- name: InsertHookSeen :execrows
INSERT INTO function_hook_seen (hook_id, webhook_id, organization_id) VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: PurgeHookSeen :execrows
DELETE FROM function_hook_seen WHERE seen_at < $1;
