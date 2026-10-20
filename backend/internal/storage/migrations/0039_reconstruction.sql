-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Reconstruction of what a user saw (SEC-142, ADR-0082): the history of what
-- each device reported running, and the signed, read-only reconstruction
-- releases built from it.

-- What a device reported at one time. A row is written when the device's
-- state differs from its previous row, so the table is the history of the
-- device's releases and presentation, not a log of every sync. bundles is a
-- JSON array of {"key", "sha256"} sorted by key and NULL when the device
-- reported only its release; channel_key is '' when the report named none.
-- theme, text_scale_percent and user_ref are NULL until the host reports
-- them. user_ref is the host's pseudonymous user identifier; it is erased
-- with the device.
CREATE TABLE device_installs (
    id                 uuid        PRIMARY KEY,
    organization_id    uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id             uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id     uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    device_id          uuid        NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    channel_key        text        NOT NULL DEFAULT '',
    release_sequence   bigint      NOT NULL,
    bundles            jsonb,
    device_class       text        NOT NULL DEFAULT '',
    locale             text        NOT NULL DEFAULT '',
    theme              text,
    text_scale_percent integer,
    user_ref           text,
    reported_at        timestamptz NOT NULL,
    CONSTRAINT device_installs_sequence CHECK (release_sequence >= 0),
    CONSTRAINT device_installs_text_scale CHECK (text_scale_percent IS NULL OR text_scale_percent BETWEEN 1 AND 1000)
);
CREATE INDEX device_installs_device ON device_installs (device_id, reported_at DESC, id DESC);
CREATE INDEX device_installs_user ON device_installs (organization_id, app_id, user_ref, reported_at DESC) WHERE user_ref IS NOT NULL;
CREATE INDEX device_installs_reported ON device_installs (organization_id, reported_at);
SELECT plux_tenant_policy('device_installs');

-- A reconstruction release. manifest is the reconstruction manifest as
-- built; the worker, which alone holds the signing keys, canonicalises and
-- signs it (SRV-052): signed and signatures stay NULL until it has. The
-- release is read-only and lives on the environment's audit channel, which
-- never has a manifest of its own, so no device is served it. subject_kind is
-- 'device' or 'user'; the rows go with the device (SEC-162).
CREATE TABLE reconstructions (
    id               uuid        PRIMARY KEY,
    organization_id  uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id           uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id   uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    channel_id       uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    device_id        uuid        NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    subject_kind     text        NOT NULL,
    moment           timestamptz NOT NULL,
    release_sequence bigint      NOT NULL,
    manifest         jsonb       NOT NULL,
    signed           bytea,
    signatures       jsonb,
    signed_at        timestamptz,
    created_by_kind  text        NOT NULL,
    created_by_id    text        NOT NULL,
    created_by       text        NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT reconstructions_subject CHECK (subject_kind IN ('device', 'user')),
    CONSTRAINT reconstructions_signed CHECK (
        (signed IS NULL AND signatures IS NULL AND signed_at IS NULL)
        OR (signed IS NOT NULL AND signatures IS NOT NULL AND signed_at IS NOT NULL))
);
CREATE INDEX reconstructions_app ON reconstructions (app_id, created_at DESC, id DESC);
CREATE INDEX reconstructions_created ON reconstructions (organization_id, created_at);
SELECT plux_tenant_policy('reconstructions');
