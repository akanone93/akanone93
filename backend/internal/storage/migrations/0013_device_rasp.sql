-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- What the runtime's self-protection reported about a device (SEC-071,
-- ADR-0057). Expand-only: both columns are new. rasp_detected_at is when
-- a detection that suggests a modified environment last arrived, null
-- while there is none; rasp_kinds are the kinds reported since the last
-- clean attestation. A fresh attestation clears both (SEC-006).
ALTER TABLE devices ADD COLUMN rasp_detected_at timestamptz;
ALTER TABLE devices ADD COLUMN rasp_kinds text[] NOT NULL DEFAULT '{}';
