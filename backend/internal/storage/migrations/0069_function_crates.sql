-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- The dependencies of function sources travel as the `.crate` archives
-- crates.io publishes, and each is stored once (FN-003, FN-016, ADR-0105).
-- Expand-only: rows and code of the previous version stay valid.

-- A published crate archive an organisation has sent. The bytes live once in
-- the object store under their SHA-256, whichever organisation sent them; a
-- row says that this organisation sent them, so each organisation is told
-- about, and builds with, only the archives it sent itself.
CREATE TABLE function_crates (
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    sha256          bytea       NOT NULL,
    object_key      text        NOT NULL,
    size_bytes      bigint      NOT NULL,
    created_by      uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, sha256),
    CONSTRAINT function_crates_sha256 CHECK (octet_length(sha256) = 32),
    CONSTRAINT function_crates_size CHECK (size_bytes > 0)
);
SELECT plux_tenant_policy('function_crates');

-- The crate archives a stored source depends on: those its Cargo.lock names,
-- recorded when the source is stored, so that every build, link and editor
-- session of the source sends exactly these to the builder.
CREATE TABLE function_source_crates (
    source_id       uuid  NOT NULL REFERENCES function_sources (id) ON DELETE CASCADE,
    organization_id uuid  NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    sha256          bytea NOT NULL,
    PRIMARY KEY (source_id, sha256),
    FOREIGN KEY (organization_id, sha256) REFERENCES function_crates (organization_id, sha256)
);
SELECT plux_tenant_policy('function_source_crates');
