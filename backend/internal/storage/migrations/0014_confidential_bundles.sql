-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Confidential bundles: a release whose sections are sealed under a
-- per-release content key, delivered wrapped to each eligible device's
-- key-agreement key (SEC-053, ADR-0055).

-- agreement_public_key is the device's key-agreement key as a JWK, the
-- way dpop_public_key is kept; NULL for a device that registered without
-- one, which therefore receives no content key.
ALTER TABLE devices ADD COLUMN agreement_public_key bytea;

-- confidential marks a release whose bundles are delivered sealed. The
-- plain versions it is made of stay as published (build once, REL-004);
-- the sealed copies below belong to this release alone.
ALTER TABLE releases ADD COLUMN confidential boolean NOT NULL DEFAULT false;

-- The content key of a confidential release, sealed under the
-- installation's key (SEC-106): signing.Sealed in its column form. It is
-- opened to wrap the key for a device, and never leaves the process.
CREATE TABLE release_content_keys (
    release_id      uuid        PRIMARY KEY REFERENCES releases (id) ON DELETE CASCADE,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    sealed          bytea       NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);
SELECT plux_tenant_policy('release_content_keys');

-- The sealed form of each bundle of a confidential release, an object in
-- object storage under bundle_sha256, the hash the manifest names
-- (plugin_key is empty for the app bundle). plain_sha256 is the version's
-- bundle hash.
CREATE TABLE release_confidential_bundles (
    release_id      uuid   NOT NULL REFERENCES releases (id) ON DELETE CASCADE,
    organization_id uuid   NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    plugin_key      text   NOT NULL,
    plain_sha256    bytea  NOT NULL,
    bundle_sha256   bytea  NOT NULL,
    size            bigint NOT NULL,
    PRIMARY KEY (release_id, plugin_key)
);
SELECT plux_tenant_policy('release_confidential_bundles');
