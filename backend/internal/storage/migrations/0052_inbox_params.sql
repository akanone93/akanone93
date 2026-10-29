-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Inbox parameters, grouping and retention (SRV-062, STU-012, P11 S0.5).
-- Expand-only (SRV-021): two columns with defaults and two indexes, so a
-- replica of the previous version keeps inserting rows.
--
-- params holds the typed values of the event, keyed by name, which Studio
-- formats in the reader's language from the event type's message. group_key
-- is equal for repeats of one event (same type, app and subject); an empty
-- key, written by a previous version, groups the row alone.
ALTER TABLE notifications
    ADD COLUMN params jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN group_key text NOT NULL DEFAULT '';
CREATE INDEX notifications_group ON notifications (organization_id, user_id, group_key, created_at DESC);

-- The retention sweep deletes the rows older than
-- notifications.retentionDays, oldest first.
CREATE INDEX notifications_age ON notifications (organization_id, created_at);
