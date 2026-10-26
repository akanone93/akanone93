-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- App and plugin metadata (P11 S3.1, plan §4.3 #13). Expand-only: nullable or
-- defaulted columns. An archived app or plugin stays readable and is left out
-- of listings unless asked for; archiving is not deletion (GOV-031 is the
-- trash). owning_team_id is cleared when its team is deleted.
ALTER TABLE apps
    ADD COLUMN description    text        NOT NULL DEFAULT '',
    ADD COLUMN icon_asset_id  text        NOT NULL DEFAULT '',
    ADD COLUMN owning_team_id uuid        REFERENCES teams (id) ON DELETE SET NULL,
    ADD COLUMN tags           text[]      NOT NULL DEFAULT '{}',
    ADD COLUMN archived_at    timestamptz;

ALTER TABLE plugins
    ADD COLUMN description    text        NOT NULL DEFAULT '',
    ADD COLUMN icon_asset_id  text        NOT NULL DEFAULT '',
    ADD COLUMN owning_team_id uuid        REFERENCES teams (id) ON DELETE SET NULL,
    ADD COLUMN tags           text[]      NOT NULL DEFAULT '{}',
    ADD COLUMN archived_at    timestamptz;

CREATE INDEX apps_owning_team ON apps (owning_team_id) WHERE owning_team_id IS NOT NULL;
