-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertLibraryItem :one
INSERT INTO library_items (id, organization_id, kind, key, document_id, name, description, template_kind, visibility, created_by, created_by_name)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'private', $9, $10)
RETURNING *;

-- An item the caller may see: those of the organisation that are not another
-- person's private item, and every public item (row-level security lets the
-- public ones of other organisations through).
-- name: GetLibraryItem :one
SELECT * FROM library_items
 WHERE id = $1 AND (visibility <> 'private' OR created_by = @actor::text);

-- name: GetLibraryItemForUpdate :one
SELECT * FROM library_items
 WHERE id = $1 AND organization_id = $2 AND (visibility <> 'private' OR created_by = @actor::text)
   FOR UPDATE;

-- name: CountLibraryItems :one
SELECT count(*) FROM library_items WHERE organization_id = $1;

-- name: ListLibraryItems :many
SELECT * FROM library_items
 WHERE kind = $1
   AND (visibility <> 'private' OR created_by = @actor::text)
   AND (@visibility::text = '' OR visibility = @visibility::text)
   AND (@query::text = '' OR key ILIKE '%' || @query::text || '%' OR name ILIKE '%' || @query::text || '%')
   AND (@after_key::text = '' OR (key, id) > (@after_key::text, @after_id::uuid))
 ORDER BY key, id
 LIMIT @page_size::int;

-- name: UpdateLibraryItem :one
UPDATE library_items SET name = $2, description = $3, visibility = $4, head_version = $5, updated_at = now()
 WHERE id = $1
RETURNING *;

-- name: InsertLibraryVersion :one
INSERT INTO library_versions (item_id, organization_id, version, content, content_sha256, note, created_by, created_by_name)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetLibraryVersion :one
SELECT * FROM library_versions WHERE item_id = $1 AND version = $2;

-- name: ListLibraryVersions :many
SELECT * FROM library_versions WHERE item_id = $1 ORDER BY version DESC;

-- name: UpsertLibraryAdoption :one
INSERT INTO library_adoptions (app_id, item_id, organization_id, version, adopted_by)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (app_id, item_id) DO UPDATE SET version = EXCLUDED.version, adopted_by = EXCLUDED.adopted_by, adopted_at = now()
RETURNING *;

-- name: ListLibraryAdoptions :many
SELECT * FROM library_adoptions WHERE app_id = $1 ORDER BY item_id;
