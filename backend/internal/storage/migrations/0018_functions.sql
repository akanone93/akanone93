-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Plux Functions on the server (FN-005, FN-006, FN-009, FN-016, FN-018,
-- ADR-0063). Every change here adds a table: nothing existing is altered
-- (DEP-030). Every table carries its organisation and is bound to it by
-- row-level security (SRV-022).

-- An uploaded source archive: the crate and its vendored dependencies.
-- The archive itself lives in the object store under object_key.
CREATE TABLE function_sources (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id  uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    name            text        NOT NULL,
    object_key      text        NOT NULL,
    sha256          bytea       NOT NULL,
    size_bytes      bigint      NOT NULL,
    created_by      uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT function_sources_name CHECK (
        length(name) <= 128 AND name ~ '^[a-z][a-zA-Z0-9]*(\.[a-z][a-zA-Z0-9]*)+$'),
    CONSTRAINT function_sources_sha256 CHECK (octet_length(sha256) = 32),
    CONSTRAINT function_sources_size CHECK (size_bytes > 0)
);
SELECT plux_tenant_policy('function_sources');
CREATE INDEX function_sources_environment ON function_sources (environment_id, name, created_at);

-- A build of a source on the builder role. A succeeded build holds the
-- module in the object store, the statement the builder's artifact is
-- described by and the signature over it (FN-009); only such a build can
-- be deployed. placement is the one the caller asked for, or '' to take
-- the one the module declares; a succeeded build holds the module's own.
CREATE TABLE function_builds (
    id                 uuid        PRIMARY KEY,
    organization_id    uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id             uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id     uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    source_id          uuid        NOT NULL REFERENCES function_sources (id) ON DELETE CASCADE,
    name               text        NOT NULL,
    placement          text        NOT NULL DEFAULT '',
    state              text        NOT NULL DEFAULT 'queued',
    diagnostics        jsonb       NOT NULL DEFAULT '[]',
    test_results       jsonb       NOT NULL DEFAULT '[]',
    module_key         text,
    module_sha256      bytea,
    abi_version        integer,
    functions_manifest jsonb,
    provenance         jsonb,
    statement          bytea,
    signature          bytea,
    key_id             text,
    created_by         uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    started_at         timestamptz,
    finished_at        timestamptz,
    CONSTRAINT function_builds_state CHECK (state IN ('queued', 'running', 'succeeded', 'failed')),
    CONSTRAINT function_builds_placement CHECK (placement IN ('', 'server', 'device')),
    CONSTRAINT function_builds_module_sha256 CHECK (module_sha256 IS NULL OR octet_length(module_sha256) = 32),
    CONSTRAINT function_builds_succeeded CHECK (
        state <> 'succeeded' OR (
            placement <> '' AND module_key IS NOT NULL AND module_sha256 IS NOT NULL
            AND abi_version IS NOT NULL AND functions_manifest IS NOT NULL AND provenance IS NOT NULL
            AND statement IS NOT NULL AND signature IS NOT NULL AND key_id IS NOT NULL))
);
SELECT plux_tenant_policy('function_builds');
CREATE INDEX function_builds_source ON function_builds (source_id, created_at);

-- A version of a function: immutable (FN-006). The number is assigned at
-- deploy, one more than the highest of the function in its app and
-- environment, under a lock that serialises the deploys of one function.
-- The statement and signature are copied from the build, so that the
-- version alone is what the function runner verifies (FN-009).
CREATE TABLE function_versions (
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id  uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    name            text        NOT NULL,
    version         integer     NOT NULL,
    build_id        uuid        NOT NULL REFERENCES function_builds (id),
    placement       text        NOT NULL,
    module_key      text        NOT NULL,
    module_sha256   bytea       NOT NULL,
    abi_version     integer     NOT NULL,
    input_schema    text        NOT NULL,
    output_schema   text        NOT NULL,
    provenance      jsonb       NOT NULL,
    statement       bytea       NOT NULL,
    signature       bytea       NOT NULL,
    key_id          text        NOT NULL,
    tests_passed    boolean     NOT NULL,
    created_by      uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (environment_id, name, version),
    CONSTRAINT function_versions_version CHECK (version > 0),
    CONSTRAINT function_versions_placement CHECK (placement IN ('server', 'device')),
    CONSTRAINT function_versions_module_sha256 CHECK (octet_length(module_sha256) = 32)
);
SELECT plux_tenant_policy('function_versions');

-- function_versions_immutable refuses every update (FN-006): a version
-- that was deployed is the same for ever, so a release that pinned it is
-- reproducible. The trigger fires for every role, the table's owner
-- included.
CREATE FUNCTION function_versions_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
    BEGIN
        RAISE EXCEPTION 'function versions are immutable (FN-006)' USING ERRCODE = 'restrict_violation';
    END;
    $$;

CREATE TRIGGER function_versions_no_update
    BEFORE UPDATE ON function_versions
    FOR EACH ROW EXECUTE FUNCTION function_versions_immutable();

-- An alias points a name at a version; unlike a version, it moves
-- (FN-006).
CREATE TABLE function_aliases (
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id  uuid        NOT NULL,
    name            text        NOT NULL,
    alias           text        NOT NULL,
    version         integer     NOT NULL,
    updated_by      uuid        REFERENCES users (id) ON DELETE SET NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (environment_id, name, alias),
    FOREIGN KEY (environment_id, name, version)
        REFERENCES function_versions (environment_id, name, version) ON DELETE CASCADE,
    CONSTRAINT function_aliases_alias CHECK (length(alias) <= 32 AND alias ~ '^[a-z][a-z0-9-]*$')
);
SELECT plux_tenant_policy('function_aliases');

-- The key-value store of a server function: one namespace per app,
-- environment and function (FN-018). Its bounds, function.kvKeys,
-- function.kvBytes and function.kvValueBytes, are enforced by the
-- service inside the transaction that writes.
CREATE TABLE function_kv (
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id  uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    function_name   text        NOT NULL,
    key             text        NOT NULL,
    value           bytea       NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (environment_id, function_name, key),
    CONSTRAINT function_kv_key CHECK (key <> '')
);
SELECT plux_tenant_policy('function_kv');
