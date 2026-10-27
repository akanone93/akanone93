-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- The library: templates and components authored in Studio (STU-080 to
-- STU-086, ADR-0098, P11 S6.2). Expand-only: three new tables and nothing
-- existing is altered, so a replica of the previous version keeps working
-- during a rolling upgrade (SRV-021).

-- An item is a template or a component of an organisation. Its content lives
-- in immutable versions; the item holds what may change without a new
-- version (name, description, visibility). document_id is the identifier the
-- content documents carry, which instances of a component refer to (SCH-030);
-- every version of an item has the same one. created_by is the author's user
-- identifier, which lets the author edit the item without library.manage.
CREATE TABLE library_items (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    kind            text        NOT NULL,
    key             text        NOT NULL,
    document_id     text        NOT NULL,
    name            text        NOT NULL,
    description     text        NOT NULL DEFAULT '',
    template_kind   text        NOT NULL DEFAULT '',
    visibility      text        NOT NULL DEFAULT 'private',
    head_version    integer     NOT NULL DEFAULT 1,
    created_by      text        NOT NULL,
    created_by_name text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT library_items_kind CHECK (kind IN ('template', 'component')),
    CONSTRAINT library_items_template_kind CHECK (
        (kind = 'template' AND template_kind IN ('page', 'flow')) OR (kind = 'component' AND template_kind = '')),
    CONSTRAINT library_items_visibility CHECK (visibility IN ('private', 'organization', 'public')),
    CONSTRAINT library_items_key_form CHECK (key ~ '^[a-z][a-z0-9-]{0,63}$'),
    CONSTRAINT library_items_unique_key UNIQUE (organization_id, kind, key),
    CONSTRAINT library_items_unique_document UNIQUE (organization_id, kind, document_id)
);
CREATE INDEX library_items_browse ON library_items (organization_id, kind, visibility, key);
SELECT plux_tenant_policy('library_items');
-- Public items are shown to the whole installation (STU-081). The policy
-- only reads: changing a row still needs the tenant policy above.
CREATE POLICY plux_public_read ON library_items FOR SELECT USING (visibility = 'public');

-- A version is immutable: the trigger below refuses any update, so a
-- component version an app adopted can never change under it (STU-085).
-- content_sha256 is the SHA-256 of content, the canonical JSON document.
CREATE TABLE library_versions (
    item_id         uuid        NOT NULL REFERENCES library_items (id) ON DELETE CASCADE,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    version         integer     NOT NULL,
    content         bytea       NOT NULL,
    content_sha256  bytea       NOT NULL,
    note            text        NOT NULL DEFAULT '',
    created_by      text        NOT NULL,
    created_by_name text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (item_id, version),
    CONSTRAINT library_versions_number CHECK (version >= 1)
);
SELECT plux_tenant_policy('library_versions');
CREATE POLICY plux_public_read ON library_versions FOR SELECT USING (
    EXISTS (SELECT 1 FROM library_items i WHERE i.id = item_id AND i.visibility = 'public'));

CREATE FUNCTION library_versions_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
    BEGIN
        RAISE EXCEPTION 'a library version is immutable' USING ERRCODE = 'integrity_constraint_violation';
    END;
    $$;
CREATE TRIGGER library_versions_immutable BEFORE UPDATE ON library_versions
    FOR EACH ROW EXECUTE FUNCTION library_versions_immutable();

-- The version of a component an app adopted (STU-085): one row per app and
-- item. item_id may name a public item of another organisation, so it has no
-- foreign key into the tenant-scoped table; the adopting call checks that it
-- can see it.
CREATE TABLE library_adoptions (
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    item_id         uuid        NOT NULL,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    version         integer     NOT NULL,
    adopted_by      text        NOT NULL,
    adopted_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (app_id, item_id),
    CONSTRAINT library_adoptions_number CHECK (version >= 1)
);
SELECT plux_tenant_policy('library_adoptions');
SELECT plux_app_policy('library_adoptions');
