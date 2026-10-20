-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Alert rules and their states (ANL-030, ADR-0080).

-- name: InsertAlertRule :one
INSERT INTO alert_rules (id, organization_id, app_id, name, metric, environment_id, filters, aggregation, window_seconds,
                         comparison, threshold, for_seconds, severity, enabled, created_by, created_at, updated_by, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $15, $16)
RETURNING *;

-- name: UpdateAlertRule :one
UPDATE alert_rules
   SET name = $3, metric = $4, environment_id = $5, filters = $6, aggregation = $7, window_seconds = $8, comparison = $9,
       threshold = $10, for_seconds = $11, severity = $12, enabled = $13, revision = revision + 1,
       updated_by = $14, updated_at = $15
 WHERE id = $1 AND organization_id = $2
RETURNING *;

-- name: GetAlertRule :one
SELECT * FROM alert_rules WHERE id = $1 AND organization_id = $2;

-- name: DeleteAlertRule :one
DELETE FROM alert_rules WHERE id = $1 AND organization_id = $2 RETURNING *;

-- name: CountAlertRules :one
SELECT count(*) FROM alert_rules WHERE organization_id = $1;

-- name: ListAlertRules :many
SELECT * FROM alert_rules
 WHERE organization_id = $1 AND (sqlc.arg(app_id)::uuid IS NULL OR app_id = sqlc.arg(app_id)::uuid)
 ORDER BY app_id, name
 LIMIT sqlc.arg(row_limit);

-- name: ListEnabledAlertRules :many
SELECT * FROM alert_rules WHERE organization_id = $1 AND enabled ORDER BY id;

-- name: EnsureAlertState :exec
INSERT INTO alert_states (rule_id, organization_id, state, since, evaluated_at)
VALUES ($1, $2, 'ok', $3, 'epoch')
ON CONFLICT (rule_id) DO NOTHING;

-- name: LockAlertState :one
SELECT * FROM alert_states WHERE rule_id = $1 FOR UPDATE;

-- name: SetAlertState :exec
UPDATE alert_states
   SET state = $2, since = $3, evaluated_at = $4, value = $5, fired_at = $6, resolved_at = $7
 WHERE rule_id = $1;

-- name: DeleteAlertState :exec
DELETE FROM alert_states WHERE rule_id = $1;

-- name: ListAlertStates :many
SELECT s.rule_id, s.state, s.since, s.evaluated_at, s.value, s.fired_at, s.resolved_at, r.app_id, r.name AS rule_name
  FROM alert_states s JOIN alert_rules r ON r.id = s.rule_id
 WHERE s.organization_id = $1
   AND (sqlc.arg(app_id)::uuid IS NULL OR r.app_id = sqlc.arg(app_id)::uuid)
   AND (sqlc.arg(state)::text = '' OR s.state = sqlc.arg(state)::text)
 ORDER BY r.app_id, r.name
 LIMIT sqlc.arg(row_limit);
