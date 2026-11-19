-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Build log #169. Whether an account that has not accepted its invitation
-- also belongs to another organisation decides two things: removing it from
-- this organisation voids the invitation only when no other organisation
-- has a stake in it, and inviting it again issues a fresh invitation only
-- then, so no organisation can take over an account another one invited.
-- The 'invitation' scope lets the service count those memberships; it only
-- reads, and the count never leaves the service. Expand-only.
CREATE POLICY plux_invitation ON memberships FOR SELECT USING (plux_in_scope('invitation'));
