-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- The push token of a device, for the silent push that makes it check its
-- control document at once (SYN-060, ADR-0077). A token is personal data:
-- it is stored only with envelope encryption (SEC-106), bound to its
-- device, and goes with the device (ON DELETE CASCADE). One token per
-- device; registering again replaces it. provider names the service that
-- delivers to it, which the sender needs without opening the token.
CREATE TABLE device_push_tokens (
    device_id       uuid        PRIMARY KEY REFERENCES devices (id) ON DELETE CASCADE,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id  uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    provider        text        NOT NULL,
    ciphertext      bytea       NOT NULL,
    wrapped_key     bytea       NOT NULL,
    key_id          text        NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT device_push_tokens_provider CHECK (provider IN ('fcm', 'apns'))
);
CREATE INDEX device_push_tokens_environment ON device_push_tokens (environment_id, device_id);
SELECT plux_tenant_policy('device_push_tokens');
