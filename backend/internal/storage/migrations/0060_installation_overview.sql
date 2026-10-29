-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- The installation page lists every organisation with its member and app
-- counts (STU-116, P11 §4.3 #22). Counting is the one place that reads
-- across the tenant wall, so it gets its own scope: 'overview' admits
-- SELECT on memberships and apps and nothing else, and the one query that
-- declares it returns counts, never a row's content. Only an installation
-- administrator reaches it (tenancy.Service.InstallationOrganizations).
CREATE POLICY plux_overview ON memberships FOR SELECT USING (plux_in_scope('overview'));
CREATE POLICY plux_overview ON apps FOR SELECT USING (plux_in_scope('overview'));
