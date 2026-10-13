-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Signing keys that an issuer generated and an operator imports, such
-- as the Apple App Attest receipt key (SEC-123, ADR-0065). Nothing
-- existing is altered (DEP-030).
--
-- The installation holds one key per role, so the table is
-- installation-wide like installation_secrets and carries no
-- organization_id. The PKCS#8 is never stored in the clear: it is
-- encrypted with AES-256-GCM under a fresh data key, and the data key is
-- wrapped by the signing backend (SEC-106, SEC-120). role, key_id and
-- team_id are bound into the ciphertext as additional data, so a row
-- whose metadata was edited fails to open.
CREATE TABLE imported_keys (
    role               text        PRIMARY KEY,
    key_id             text        NOT NULL,
    team_id            text        NOT NULL,
    wrapped_data_key   bytea       NOT NULL,
    wrap_key_id        text        NOT NULL,
    nonce              bytea       NOT NULL,
    ciphertext         bytea       NOT NULL,
    public_key_sha256  bytea       NOT NULL,
    created_at         timestamptz NOT NULL,
    CONSTRAINT imported_keys_role CHECK (role ~ '^[a-z0-9][a-z0-9-]{0,63}$'),
    CONSTRAINT imported_keys_digest CHECK (octet_length(public_key_sha256) = 32)
);
