-- name: CreatePlugin :one
INSERT INTO plugins (id, organization_id, app_id, key, name)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPlugin :one
SELECT * FROM plugins WHERE id = $1 AND deleted_at IS NULL;

-- name: GetPluginIncludingDeleted :one
SELECT * FROM plugins WHERE id = $1;

-- name: GetPluginByKey :one
SELECT * FROM plugins WHERE app_id = $1 AND key = $2 AND deleted_at IS NULL;

-- name: ListPlugins :many
SELECT * FROM plugins
 WHERE app_id = $1 AND deleted_at IS NULL
   AND (key, id) > (sqlc.arg(after_key)::text, sqlc.arg(after_id)::uuid)
 ORDER BY key, id
 LIMIT sqlc.arg(page_size);

-- name: UpdatePluginMetadata :one
UPDATE plugins
   SET description    = CASE WHEN sqlc.arg(set_description)::boolean THEN sqlc.arg(description)::text ELSE description END,
       icon_asset_id  = CASE WHEN sqlc.arg(set_icon)::boolean THEN sqlc.arg(icon_asset_id)::text ELSE icon_asset_id END,
       owning_team_id = CASE WHEN sqlc.arg(set_team)::boolean THEN sqlc.narg(owning_team_id)::uuid ELSE owning_team_id END,
       tags           = CASE WHEN sqlc.arg(set_tags)::boolean THEN sqlc.arg(tags)::text[] ELSE tags END,
       archived_at    = CASE WHEN sqlc.arg(set_archived)::boolean THEN (CASE WHEN sqlc.arg(archived)::boolean THEN coalesce(archived_at, now()) END) ELSE archived_at END,
       name           = CASE WHEN sqlc.arg(name)::text = '' THEN name ELSE sqlc.arg(name)::text END,
       updated_at     = now()
 WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SummarisePlugins :many
-- One row per live plugin of an app for the app canvas (plan §4.3 #14): the
-- draft, its screens, the newest published version, and the lock. Bounded by
-- app.plugins.
SELECT p.id, p.key, p.name, p.latest_version, p.archived_at, p.owning_team_id,
       d.revision AS draft_revision, d.updated_at AS draft_updated_at,
       (SELECT count(*) FROM documents x WHERE x.draft_id = d.id AND x.kind = 'page' AND x.deleted_at IS NULL)::bigint AS screens,
       coalesce((SELECT v.bundle_size FROM plugin_versions v WHERE v.plugin_id = p.id ORDER BY v.version DESC LIMIT 1), 0)::bigint AS bundle_size,
       (SELECT v.created_at FROM plugin_versions v WHERE v.plugin_id = p.id ORDER BY v.version DESC LIMIT 1) AS published_at,
       l.holder_display AS lock_holder, l.expires_at AS lock_expires_at
  FROM plugins p
  JOIN drafts d ON d.plugin_id = p.id
  LEFT JOIN locks l ON l.draft_id = d.id AND l.expires_at > now()
 WHERE p.app_id = $1 AND p.deleted_at IS NULL
 ORDER BY p.key
 LIMIT sqlc.arg(max_rows);

-- name: ListPageEntities :many
-- The entity ID of every page and the plugin that holds it, for resolving
-- the links between plugins.
SELECT x.entity_id, d.plugin_id
  FROM documents x JOIN drafts d ON d.id = x.draft_id
 WHERE d.app_id = $1 AND d.plugin_id IS NOT NULL AND x.kind = 'page' AND x.deleted_at IS NULL AND x.entity_id IS NOT NULL;

-- name: ListKilledPlugins :many
-- The plugins any channel of the app's environments currently switches off.
SELECT DISTINCT unnest(c.kill_switch_plugins)::text AS plugin_key
  FROM channel_controls c
  JOIN channels ch ON ch.id = c.channel_id
  JOIN environments e ON e.id = ch.environment_id
 WHERE e.app_id = $1;

-- name: ListAllPlugins :many
SELECT * FROM plugins WHERE app_id = $1 AND deleted_at IS NULL ORDER BY key;

-- name: CountPlugins :one
SELECT count(*) FROM plugins WHERE app_id = $1 AND deleted_at IS NULL;

-- name: UpdatePlugin :one
UPDATE plugins SET name = $2, updated_at = now() WHERE id = $1 AND deleted_at IS NULL RETURNING *;

-- name: SoftDeletePlugin :one
UPDATE plugins SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL RETURNING *;

-- name: RestorePlugin :one
UPDATE plugins SET deleted_at = NULL, updated_at = now() WHERE id = $1 RETURNING *;

-- name: PurgePlugin :execrows
DELETE FROM plugins WHERE id = $1 AND deleted_at IS NOT NULL;

-- name: SetPluginVersion :exec
UPDATE plugins SET latest_version = $2, updated_at = now() WHERE id = $1;

-- name: CreateDraft :one
INSERT INTO drafts (id, organization_id, app_id, plugin_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (app_id, plugin_id) DO UPDATE SET updated_at = drafts.updated_at
RETURNING *;

-- name: GetDraft :one
SELECT * FROM drafts WHERE app_id = $1 AND plugin_id IS NOT DISTINCT FROM sqlc.narg(plugin_id)::uuid;

-- name: GetDraftForUpdate :one
SELECT * FROM drafts WHERE id = $1 FOR UPDATE;

-- name: ListDrafts :many
-- Every draft of an app whose plugin is not deleted, the app's own first.
SELECT d.* FROM drafts d
  LEFT JOIN plugins p ON p.id = d.plugin_id
 WHERE d.app_id = $1 AND (d.plugin_id IS NULL OR p.deleted_at IS NULL)
 ORDER BY d.plugin_id NULLS FIRST;

-- name: AdvanceDraft :one
UPDATE drafts
   SET revision = revision + sqlc.arg(revisions)::bigint, snapshots = snapshots + 1, updated_at = now()
 WHERE id = $1
RETURNING *;

-- name: PutBlob :exec
INSERT INTO blobs (organization_id, sha256, size, content)
VALUES ($1, $2, $3, $4)
ON CONFLICT (organization_id, sha256) DO NOTHING;

-- name: GetBlob :one
SELECT * FROM blobs WHERE organization_id = $1 AND sha256 = $2;

-- name: DeleteUnreferencedBlobs :execrows
DELETE FROM blobs b
 WHERE b.organization_id = $1
   AND b.created_at < now() - interval '1 day'
   AND NOT EXISTS (SELECT 1 FROM documents d WHERE d.organization_id = b.organization_id AND d.sha256 = b.sha256)
   AND NOT EXISTS (SELECT 1 FROM snapshot_documents s WHERE s.organization_id = b.organization_id AND s.sha256 = b.sha256);

-- name: GetDocument :one
SELECT * FROM documents WHERE draft_id = $1 AND path = $2 AND deleted_at IS NULL;

-- name: GetDocumentForUpdate :one
SELECT * FROM documents WHERE draft_id = $1 AND path = $2 AND deleted_at IS NULL FOR UPDATE;

-- name: GetDocumentByID :one
SELECT * FROM documents WHERE id = $1;

-- name: GetDocumentByEntity :one
SELECT d.* FROM documents d
  JOIN drafts dr ON dr.id = d.draft_id
 WHERE d.entity_id = $1 AND d.kind = $2 AND d.deleted_at IS NULL;

-- name: ListDocuments :many
SELECT * FROM documents
 WHERE draft_id = $1 AND deleted_at IS NULL AND path > sqlc.arg(after_path)::text
 ORDER BY path
 LIMIT sqlc.arg(page_size);

-- name: ListAllDocuments :many
SELECT * FROM documents WHERE draft_id = $1 AND deleted_at IS NULL ORDER BY path;

-- name: ListDocumentsOfKind :many
SELECT d.*, dr.plugin_id FROM documents d
  JOIN drafts dr ON dr.id = d.draft_id
  LEFT JOIN plugins p ON p.id = dr.plugin_id
 WHERE dr.app_id = $1 AND d.kind = $2 AND d.deleted_at IS NULL
   AND (dr.plugin_id IS NULL OR p.deleted_at IS NULL)
   AND (sqlc.narg(plugin_id)::uuid IS NULL OR dr.plugin_id = sqlc.narg(plugin_id)::uuid)
   AND d.path > sqlc.arg(after_path)::text
 ORDER BY d.path
 LIMIT sqlc.arg(page_size);

-- name: InsertDocument :one
INSERT INTO documents (id, organization_id, draft_id, path, kind, entity_id, entity_key, sha256, revision,
                       updated_by_kind, updated_by_id, updated_by, search_text)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, $9, $10, $11, $12)
RETURNING *;

-- name: UpdateDocument :one
UPDATE documents
   SET kind = $2, entity_id = $3, entity_key = $4, sha256 = $5, revision = revision + 1,
       updated_by_kind = $6, updated_by_id = $7, updated_by = $8, search_text = $9, updated_at = now()
 WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteDocument :one
UPDATE documents SET deleted_at = now(), revision = revision + 1 WHERE id = $1 AND deleted_at IS NULL RETURNING *;

-- name: HardDeleteDocument :execrows
DELETE FROM documents WHERE id = $1;

-- name: RestoreDocument :one
UPDATE documents SET deleted_at = NULL, revision = revision + 1, updated_at = now() WHERE id = $1 RETURNING *;

-- name: CreateSnapshot :one
INSERT INTO snapshots (id, organization_id, draft_id, sequence, revision, reason, applied,
                       actor_kind, actor_id, actor_display, keep, editor_session_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: AddSnapshotDocument :exec
INSERT INTO snapshot_documents (snapshot_id, organization_id, path, kind, sha256, carried, inverse_patch, step_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (snapshot_id, path) DO NOTHING;

-- name: GetSnapshot :one
SELECT * FROM snapshots WHERE id = $1;

-- name: ListSnapshots :many
SELECT * FROM snapshots
 WHERE draft_id = $1 AND sequence < sqlc.arg(before_sequence)::bigint
 ORDER BY sequence DESC
 LIMIT sqlc.arg(page_size);

-- name: ListEditorSessionEntries :many
-- The entries of an editor session on a draft, newest first, for Studio's
-- undo rebuild (STU-069): every applied write it made that kept its
-- inverse patch.
SELECT s.id, s.sequence, s.revision, s.created_at, sd.path, sd.inverse_patch, sd.step_id
  FROM snapshots s
  JOIN snapshot_documents sd ON sd.snapshot_id = s.id
 WHERE s.draft_id = $1
   AND s.editor_session_id = sqlc.arg(editor_session_id)::text
   AND s.applied AND NOT sd.carried AND sd.inverse_patch IS NOT NULL
   AND s.sequence < sqlc.arg(before_sequence)::bigint
 ORDER BY s.sequence DESC, sd.path
 LIMIT sqlc.arg(page_size);

-- name: PruneEditorSessionEntries :exec
-- Clears the inverse patches of an editor session's writes on a draft
-- beyond its newest keep (studio.undoDepth).
UPDATE snapshot_documents SET inverse_patch = NULL
 WHERE inverse_patch IS NOT NULL
   AND snapshot_id IN (SELECT id FROM snapshots
                        WHERE draft_id = $1 AND editor_session_id = sqlc.arg(editor_session_id)::text
                        ORDER BY sequence DESC
                        OFFSET sqlc.arg(keep)::bigint);

-- name: ListSnapshotPaths :many
SELECT path FROM snapshot_documents WHERE snapshot_id = $1 AND NOT carried ORDER BY path;

-- name: SnapshotState :many
-- The state of a draft at a snapshot: for each path, its latest entry
-- among the applied snapshots up to and including this one. A preserved
-- snapshot's state is its own entries over the state it was based on.
SELECT DISTINCT ON (sd.path) sd.path, sd.kind, sd.sha256
  FROM snapshot_documents sd
  JOIN snapshots s ON s.id = sd.snapshot_id
 WHERE s.draft_id = sqlc.arg(draft_id)::uuid
   AND s.sequence <= sqlc.arg(sequence)::bigint
   AND (s.applied OR s.id = sqlc.arg(snapshot_id)::uuid)
 ORDER BY sd.path, s.sequence DESC;

-- name: KeepSnapshot :exec
UPDATE snapshots SET keep = true WHERE id = $1;

-- name: ListExpiredSnapshots :many
SELECT * FROM snapshots
 WHERE draft_id = $1 AND NOT keep AND created_at < sqlc.arg(before)::timestamptz
 ORDER BY sequence;

-- name: DeleteSnapshot :exec
DELETE FROM snapshots WHERE id = $1;

-- name: ListDraftsWithExpiredSnapshots :many
SELECT DISTINCT draft_id FROM snapshots WHERE NOT keep AND created_at < sqlc.arg(before)::timestamptz LIMIT sqlc.arg(page_size);

-- name: GetLockForUpdate :one
SELECT * FROM locks WHERE draft_id = $1 FOR UPDATE;

-- name: GetLock :one
SELECT * FROM locks WHERE draft_id = $1;

-- name: PutLock :one
INSERT INTO locks (draft_id, organization_id, holder_kind, holder_id, holder_display, session,
                   expires_at, previous_session, previous_holder, last_edit_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (draft_id) DO UPDATE
   SET holder_kind = EXCLUDED.holder_kind, holder_id = EXCLUDED.holder_id,
       holder_display = EXCLUDED.holder_display, session = EXCLUDED.session,
       acquired_at = now(), heartbeat_at = now(), expires_at = EXCLUDED.expires_at,
       previous_session = EXCLUDED.previous_session, previous_holder = EXCLUDED.previous_holder,
       requested_by = '[]', last_edit_at = EXCLUDED.last_edit_at
RETURNING *;

-- name: TouchLock :exec
UPDATE locks SET last_edit_at = $3 WHERE draft_id = $1 AND session = $2;

-- name: RenewLock :one
UPDATE locks SET heartbeat_at = now(), expires_at = $2 WHERE draft_id = $1 RETURNING *;

-- name: DeleteLock :execrows
DELETE FROM locks WHERE draft_id = $1;

-- name: RequestLock :one
UPDATE locks
   SET requested_by = CASE WHEN requested_by @> sqlc.arg(requester)::jsonb THEN requested_by
                           ELSE requested_by || sqlc.arg(requester)::jsonb END
 WHERE draft_id = $1
RETURNING *;

-- name: GetDraftByID :one
SELECT * FROM drafts WHERE id = $1;

-- name: ListSnapshotsFrom :many
SELECT * FROM snapshots WHERE draft_id = $1 AND sequence > $2 ORDER BY sequence LIMIT sqlc.arg(page_size);

-- name: ListKeptSnapshotsBefore :many
SELECT * FROM snapshots WHERE draft_id = $1 AND keep AND sequence < $2 ORDER BY sequence;

-- name: NewestAppliedSnapshot :one
SELECT * FROM snapshots WHERE draft_id = $1 AND applied ORDER BY sequence DESC LIMIT 1;

-- name: SnapshotAtRevision :one
SELECT * FROM snapshots
 WHERE draft_id = $1 AND applied AND revision = sqlc.arg(revision)::bigint
 ORDER BY sequence DESC
 LIMIT 1;
