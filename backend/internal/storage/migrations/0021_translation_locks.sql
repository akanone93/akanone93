-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- A draft may now carry several editing locks: its own, and one per locale
-- of its translations, so that translators never block the theme or app.json
-- (I18N-008, ADR-0068). scope is empty for the draft lock (SRV-040) and
-- translations/<locale> for a locale's lock. Existing rows keep scope ''.
ALTER TABLE locks ADD COLUMN scope text NOT NULL DEFAULT '';
ALTER TABLE locks DROP CONSTRAINT locks_pkey;
ALTER TABLE locks ADD PRIMARY KEY (draft_id, scope);
