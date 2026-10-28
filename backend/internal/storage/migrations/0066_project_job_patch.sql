-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- A project job may build a patch (the files that changed since the last
-- generation) as well as the whole project (GEN-004). Expand-only: the
-- check admits one more value, so rows and code of the previous version
-- stay valid.
ALTER TABLE project_jobs DROP CONSTRAINT project_jobs_format;
ALTER TABLE project_jobs ADD CONSTRAINT project_jobs_format CHECK (format IN ('zip', 'patch'));
