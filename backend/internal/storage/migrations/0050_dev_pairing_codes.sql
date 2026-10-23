-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Pairing codes (DEV-002, P10 D8). Expand-only: a nullable column and a new
-- table.
--
-- A person may type the short code of a pairing token instead of opening its
-- link: eight Crockford base32 characters of the token's SHA-256. The code is
-- unique among the unconsumed tokens, so the device names one token with it;
-- once consumed, a code may come round again.
ALTER TABLE dev_pairing_tokens ADD COLUMN code text;
ALTER TABLE dev_pairing_tokens
    ADD CONSTRAINT dev_pairing_tokens_code CHECK (code IS NULL OR code ~ '^[0-9A-HJKMNP-TV-Z]{8}$');
CREATE UNIQUE INDEX dev_pairing_tokens_code_open ON dev_pairing_tokens (code) WHERE consumed_at IS NULL;

-- dev_pairing_attempts holds a device's recent pairing-code attempts. An
-- attempt is recorded before the code is tried and removed when it pairs, so
-- what remains are the failures that devSession.pairingCodeAttempts bounds per
-- device in a ten-minute window. Rows older than the window are deleted as the
-- device makes new attempts.
CREATE TABLE dev_pairing_attempts (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    device_id       text        NOT NULL,
    at              timestamptz NOT NULL
);
CREATE INDEX dev_pairing_attempts_device ON dev_pairing_attempts (organization_id, device_id, at);
SELECT plux_tenant_policy('dev_pairing_attempts');
