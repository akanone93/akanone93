-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- The editing lock of one locale of an app's translations (I18N-008,
-- ADR-0068). It is held beside the draft's own lock (locks, SRV-040), so a
-- translator never blocks the theme or app.json. The columns are those of
-- locks; the table is new and locks is untouched, so a replica of the
-- previous version keeps working during a rolling upgrade (SRV-021).
CREATE TABLE translation_locks (
    draft_id         uuid        NOT NULL REFERENCES drafts (id) ON DELETE CASCADE,
    locale           text        NOT NULL,
    organization_id  uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    holder_kind      text        NOT NULL,
    holder_id        text        NOT NULL,
    holder_display   text        NOT NULL DEFAULT '',
    session          text        NOT NULL,
    acquired_at      timestamptz NOT NULL DEFAULT now(),
    heartbeat_at     timestamptz NOT NULL DEFAULT now(),
    expires_at       timestamptz NOT NULL,
    previous_session text        NOT NULL DEFAULT '',
    previous_holder  text        NOT NULL DEFAULT '',
    requested_by     jsonb       NOT NULL DEFAULT '[]',
    PRIMARY KEY (draft_id, locale)
);
SELECT plux_tenant_policy('translation_locks');
