-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Crate archives an organisation no longer uses are removed (ADR-0105, build
-- log #163). Expand-only: rows and code of the previous version stay valid.

-- When the organisation last needed the archive: when it was sent, when a
-- deploy asked about it, and when a stored source named it. An archive no
-- source names and nobody has needed for a while is removed.
ALTER TABLE function_crates ADD COLUMN last_used_at timestamptz NOT NULL DEFAULT now();

-- The 'crate_store' scope lets the clean-up see whether any organisation
-- still holds an archive before it removes the bytes all organisations share
-- in the object store. It only reads, and only in that scope.
CREATE POLICY function_crates_store ON function_crates FOR SELECT USING (plux_in_scope('crate_store'));
