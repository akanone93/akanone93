-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Studio preferences (STU-014, STU-058, P11 S0.5). Expand-only: a new table.
--
-- user_preferences holds one row per user and organisation: the user's Studio
-- preferences there, as the JSON form of plux.v1.Preferences. A user without a
-- row has the defaults. studio.preferencesBytes bounds the JSON.
CREATE TABLE user_preferences (
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    preferences     jsonb       NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id)
);
SELECT plux_tenant_policy('user_preferences');
