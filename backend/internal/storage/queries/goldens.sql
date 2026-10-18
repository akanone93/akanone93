-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertGoldenRun :exec
INSERT INTO golden_runs (id, organization_id, app_id, plugin_key, bundle_sha256, app_sha256, canonical, renderer, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: InsertGoldenCase :exec
INSERT INTO golden_cases (run_id, case_id, organization_id, frame, locale, theme, text_scale, image_sha256, image_bytes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetGoldenRun :one
SELECT * FROM golden_runs WHERE id = $1 AND app_id = $2;

-- The newest canonical run for the exact bundles; the publish gate reads it
-- (TST-003).
-- name: LatestCanonicalGoldenRun :one
SELECT * FROM golden_runs
 WHERE app_id = $1 AND plugin_key = $2 AND bundle_sha256 = $3 AND app_sha256 = $4 AND canonical
 ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: ListGoldenBaselines :many
SELECT case_id, image_sha256 FROM golden_baselines WHERE app_id = $1 AND plugin_key = $2 ORDER BY case_id;

-- name: GoldenBaselineBytes :one
SELECT COALESCE(SUM(image_bytes), 0)::bigint AS total FROM golden_baselines WHERE app_id = $1 AND plugin_key = $2;

-- name: UpsertGoldenBaseline :exec
INSERT INTO golden_baselines (app_id, plugin_key, case_id, organization_id, image_sha256, image_bytes, approved_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (app_id, plugin_key, case_id) DO UPDATE
   SET image_sha256 = EXCLUDED.image_sha256, image_bytes = EXCLUDED.image_bytes,
       approved_by = EXCLUDED.approved_by, approved_at = now();

-- The cases of a run that differ from the baseline now: no baseline, or
-- another image. Approving a case, from this run or another, takes it out.
-- name: ListGoldenDifferences :many
SELECT c.case_id, c.frame, c.locale, c.theme, c.text_scale, c.image_sha256, c.image_bytes,
       b.image_sha256 AS baseline_sha256
  FROM golden_cases c
  JOIN golden_runs r ON r.id = c.run_id
  LEFT JOIN golden_baselines b
    ON b.app_id = r.app_id AND b.plugin_key = r.plugin_key AND b.case_id = c.case_id
 WHERE c.run_id = $1 AND b.image_sha256 IS DISTINCT FROM c.image_sha256
 ORDER BY c.case_id;

-- name: CountGoldenCases :one
SELECT count(*)::bigint AS total FROM golden_cases WHERE run_id = $1;

-- name: GoldenBaselineSize :one
SELECT image_bytes FROM golden_baselines WHERE app_id = $1 AND plugin_key = $2 AND case_id = $3;
