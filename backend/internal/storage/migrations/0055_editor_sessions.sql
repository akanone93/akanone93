-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Editor sessions for Studio's undo (P11 S2.4, STU-069, ADR-0095, contracts
-- #4 and #25). Expand-only: columns with defaults and a partial index.
--
-- editor_session_id tags a snapshot with the Studio tab that wrote it; it is
-- empty for writes made outside Studio. inverse_patch is the RFC 6902 patch
-- that turns the document a tagged write produced back into the one it
-- replaced; after a reload Studio rebuilds its undo stack from a session's
-- entries. Only the newest studio.undoDepth entries of a session and draft
-- keep their inverse patch; older ones are cleared on write, and the purge
-- of snapshots removes the rest with their snapshot.
ALTER TABLE snapshots ADD COLUMN editor_session_id text NOT NULL DEFAULT '';
ALTER TABLE snapshots
    ADD CONSTRAINT snapshots_editor_session CHECK (length(editor_session_id) <= 128);
CREATE INDEX snapshots_editor_session ON snapshots (draft_id, editor_session_id, sequence DESC)
    WHERE editor_session_id <> '';
-- step_id names the Studio undo step of a tagged write, so that the rebuild
-- regroups a step spanning several writes and documents.
ALTER TABLE snapshot_documents ADD COLUMN inverse_patch jsonb;
ALTER TABLE snapshot_documents ADD COLUMN step_id text NOT NULL DEFAULT '';
ALTER TABLE snapshot_documents
    ADD CONSTRAINT snapshot_documents_step CHECK (length(step_id) <= 128);
