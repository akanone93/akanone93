-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- The reports a release carries (A11Y-008, I18N-009): one canonical JSON
-- document per kind, produced by the compiler when the release is created
-- and immutable afterwards. The table is new and nothing existing is
-- altered, so a replica of the previous version keeps working during a
-- rolling upgrade (SRV-021).
CREATE TABLE release_reports (
    release_id      uuid  NOT NULL REFERENCES releases (id) ON DELETE CASCADE,
    kind            text  NOT NULL,
    organization_id uuid  NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    content         bytea NOT NULL,
    PRIMARY KEY (release_id, kind)
);
SELECT plux_tenant_policy('release_reports');
