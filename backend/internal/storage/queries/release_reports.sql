-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertReleaseReport :exec
INSERT INTO release_reports (release_id, kind, organization_id, content) VALUES ($1, $2, $3, $4);

-- name: GetReleaseReport :one
SELECT content FROM release_reports WHERE release_id = $1 AND kind = $2;

-- name: CopyReleaseReports :exec
INSERT INTO release_reports (release_id, kind, organization_id, content)
SELECT sqlc.arg(release_id)::uuid, earlier.kind, earlier.organization_id, earlier.content
  FROM release_reports earlier WHERE earlier.release_id = sqlc.arg(earlier_id)::uuid;
