-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- The holder's last edit on a lock (P11 S2.3, SRV-041, plan §4.3 #16).
-- Expand-only: one column with a default on each lock table.
--
-- last_edit_at is the time of the holder's last accepted write under the
-- lock, set when the lock is taken and on every write. Another user may
-- take the lock over only when it is studio.lockTakeoverIdleMinutes old,
-- or when the lease has lapsed (L-1). Locks held when this migration runs
-- count from now.
ALTER TABLE locks ADD COLUMN last_edit_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE translation_locks ADD COLUMN last_edit_at timestamptz NOT NULL DEFAULT now();
