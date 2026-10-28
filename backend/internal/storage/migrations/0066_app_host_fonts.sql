-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- The font files of the host app that an app lists for Studio's canvas
-- (STU-006): a JSON array of {family, assetId}, bounded by app.hostFonts.
-- Expand-only: a defaulted column on apps, which already has its tenant
-- policy.
ALTER TABLE apps
    ADD COLUMN host_fonts jsonb NOT NULL DEFAULT '[]'::jsonb;
