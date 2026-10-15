-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Plux Functions at run time (FN-006, FN-011, FN-012, FN-020, ADR-0063,
-- ADR-0064). Every change here adds a table: nothing existing is altered
-- (DEP-030). Every table carries its organisation and is bound to it by
-- row-level security (SRV-022).

-- The function document a build was made from (spec §16): its
-- capabilities, limits, required assurance, attestation and SCA flags,
-- environment variables and test cases. The builder reads it from the
-- uploaded source; a version takes the declaration of its build, so what
-- a version may do is as immutable as its module.
CREATE TABLE function_declarations (
    build_id        uuid        PRIMARY KEY REFERENCES function_builds (id) ON DELETE CASCADE,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    declaration     jsonb       NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT function_declarations_object CHECK (jsonb_typeof(declaration) = 'object')
);
SELECT plux_tenant_policy('function_declarations');

-- The resolved version of every function a release's plugins reference
-- (FN-006): a release's behaviour is reproducible, and a device may invoke
-- only a version that an active release of its app pins (plan p7 §2.5 E5).
CREATE TABLE release_functions (
    release_id      uuid        NOT NULL REFERENCES releases (id) ON DELETE CASCADE,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id  uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    name            text        NOT NULL,
    version         integer     NOT NULL,
    module_sha256   bytea       NOT NULL,
    placement       text        NOT NULL,
    PRIMARY KEY (release_id, name),
    CONSTRAINT release_functions_version CHECK (version > 0),
    CONSTRAINT release_functions_module_sha256 CHECK (octet_length(module_sha256) = 32),
    CONSTRAINT release_functions_placement CHECK (placement IN ('server', 'device'))
);
SELECT plux_tenant_policy('release_functions');
CREATE INDEX release_functions_lookup ON release_functions (environment_id, name, version);
