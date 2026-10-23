-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Full-text search over apps, plugins and documents (SRV-061, ADR-0082)
-- and the per-app Git export (GOV-011). Everything here is additive: one
-- immutable function, expression indexes, a column with a constant default
-- and two new tables, so a replica of the previous version keeps working
-- during a rolling upgrade (SRV-021).

-- plux_search_vector is the one definition of what a searchable row looks
-- like. It uses the language-neutral 'simple' configuration, so no stemming
-- or stop word list prefers one language, and it turns the separators of
-- paths and file names into blanks, so "plugins/loans/pages/home.page.json"
-- is found by "loans", "home" or "page". The same expression is the index
-- expression and the one every search query repeats, which is what lets
-- the planner use the index.
CREATE FUNCTION plux_search_vector(a text, b text, c text) RETURNS tsvector
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$
        SELECT to_tsvector('simple', translate(a || ' ' || b || ' ' || c, '/._', '   '))
    $$;

COMMENT ON FUNCTION plux_search_vector(text, text, text) IS
    'The text-search vector of a searchable row: three text columns, separators blanked (SRV-061).';

CREATE INDEX apps_search ON apps USING gin (plux_search_vector(name, key, ''))
    WHERE deleted_at IS NULL;
CREATE INDEX plugins_search ON plugins USING gin (plux_search_vector(name, key, ''))
    WHERE deleted_at IS NULL;

-- search_text is what a document contributes beyond its path and key: its
-- name, title and description, or, for a translation key catalogue, every
-- key with its description. The document service sets it when it writes a
-- document, in the same statement, so the index is never behind the
-- content. A document written before this migration is indexed by its path
-- and key only, until its next write.
ALTER TABLE documents ADD COLUMN search_text text NOT NULL DEFAULT '';

CREATE INDEX documents_search ON documents
    USING gin (plux_search_vector(path, entity_key, search_text))
    WHERE deleted_at IS NULL
      AND kind IN ('page', 'component', 'template', 'translationKeys', 'function');

-- The Git export setting of an app (GOV-011). It is off until an app
-- manager turns it on. The credential is not here: it names a secret of
-- an environment, which the secret store holds under envelope encryption
-- (SEC-106), and the secret is opened only by the export job.
CREATE TABLE git_settings (
    app_id                    uuid        PRIMARY KEY REFERENCES apps (id) ON DELETE CASCADE,
    organization_id           uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    enabled                   boolean     NOT NULL DEFAULT false,
    remote_url                text        NOT NULL,
    branch                    text        NOT NULL DEFAULT 'main',
    username                  text        NOT NULL DEFAULT '',
    credential_environment_id uuid        REFERENCES environments (id) ON DELETE SET NULL,
    credential_key            text        NOT NULL DEFAULT '',
    updated_by                uuid        REFERENCES users (id) ON DELETE SET NULL,
    updated_at                timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT git_settings_https CHECK (remote_url ~ '^https://[^/@]+/')
);
SELECT plux_tenant_policy('git_settings');

-- One row per export: the publish it exports (version_id is the published
-- version's identifier, the release identifier in the commit message), the
-- commit it became and how often the push was tried. The unique key makes
-- an export idempotent per publish (GOV-011). error_code is a PLX code and
-- never carries a remote's response, which could echo a credential.
CREATE TABLE git_exports (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    version_id      uuid        NOT NULL,
    state           text        NOT NULL DEFAULT 'pending',
    commit_sha      text        NOT NULL DEFAULT '',
    attempts        integer     NOT NULL DEFAULT 0,
    error_code      integer     NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (app_id, version_id),
    CONSTRAINT git_exports_state CHECK (state IN ('pending', 'pushed', 'failed'))
);
CREATE INDEX git_exports_app ON git_exports (app_id, created_at DESC);
SELECT plux_tenant_policy('git_exports');
