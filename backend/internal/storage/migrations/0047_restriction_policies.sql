-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Restriction policies, the permissions version and the second barrier of
-- row-level security (GOV-006, SEC-102, ADR-0086).
--
-- A restriction is a ceiling: it removes permissions from an organisation,
-- a team or a person whatever their grants say. Its rows are immutable; a
-- change is a delete and a create, both audited by the service. A subject
-- that is a team or a user is a real reference, so that deleting the team
-- or the user deletes their restrictions with them; subject_id is the
-- generated union of the two for the reader of the table.
CREATE TABLE restriction_policies (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    subject_kind    text        NOT NULL,
    team_id         uuid        REFERENCES teams (id) ON DELETE CASCADE,
    user_id         uuid        REFERENCES users (id) ON DELETE CASCADE,
    subject_id      uuid        GENERATED ALWAYS AS (COALESCE(team_id, user_id)) STORED,
    -- patterns are catalogue permission names or the wildcards '*',
    -- '<area>.*' and '<area>.<feature>.*', validated at write time.
    patterns        text[]      NOT NULL,
    app_id          uuid        REFERENCES apps (id) ON DELETE CASCADE,
    -- environment is the key of an environment of app_id.
    environment     text,
    reason          text        NOT NULL DEFAULT '',
    created_by      uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT restriction_policies_subject CHECK (
        (subject_kind = 'organization' AND team_id IS NULL AND user_id IS NULL)
        OR (subject_kind = 'team' AND team_id IS NOT NULL AND user_id IS NULL)
        OR (subject_kind = 'user' AND team_id IS NULL AND user_id IS NOT NULL)
    ),
    CONSTRAINT restriction_policies_patterns CHECK (cardinality(patterns) >= 1),
    CONSTRAINT restriction_policies_environment CHECK (
        environment IS NULL OR (app_id IS NOT NULL AND environment ~ '^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$')
    )
);

CREATE INDEX restriction_policies_org ON restriction_policies (organization_id, id);
CREATE INDEX restriction_policies_subject ON restriction_policies (organization_id, subject_id);
SELECT plux_tenant_policy('restriction_policies');

CREATE OR REPLACE FUNCTION plux_restriction_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
    BEGIN
        RAISE EXCEPTION 'restriction_policies rows are immutable; delete and create instead'
            USING ERRCODE = 'restrict_violation';
    END;
    $$;

CREATE TRIGGER restriction_policies_immutable
    BEFORE UPDATE ON restriction_policies
    FOR EACH ROW EXECUTE FUNCTION plux_restriction_immutable();

-- The permissions version of an organisation: a counter that rises in the
-- same transaction as any change to what a person may do there, so that a
-- cached answer can tell that it is stale (ADR-0086). A trigger on every
-- table the resolver reads makes the rule hold for every writer, the
-- service layer, SCIM and directory synchronisation alike, with no call
-- site to forget.
ALTER TABLE organizations ADD COLUMN permissions_version bigint NOT NULL DEFAULT 0;

CREATE OR REPLACE FUNCTION plux_bump_permissions_version() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
    BEGIN
        UPDATE organizations
           SET permissions_version = permissions_version + 1
         WHERE id = CASE WHEN TG_OP = 'DELETE' THEN OLD.organization_id ELSE NEW.organization_id END;
        RETURN NULL;
    END;
    $$;

CREATE TRIGGER role_grants_permissions_version
    AFTER INSERT OR UPDATE OR DELETE ON role_grants
    FOR EACH ROW EXECUTE FUNCTION plux_bump_permissions_version();
CREATE TRIGGER app_access_permissions_version
    AFTER INSERT OR UPDATE OR DELETE ON app_access
    FOR EACH ROW EXECUTE FUNCTION plux_bump_permissions_version();
CREATE TRIGGER custom_roles_permissions_version
    AFTER INSERT OR UPDATE OR DELETE ON custom_roles
    FOR EACH ROW EXECUTE FUNCTION plux_bump_permissions_version();
CREATE TRIGGER memberships_permissions_version
    AFTER INSERT OR UPDATE OR DELETE ON memberships
    FOR EACH ROW EXECUTE FUNCTION plux_bump_permissions_version();
CREATE TRIGGER restriction_policies_permissions_version
    AFTER INSERT OR DELETE ON restriction_policies
    FOR EACH ROW EXECUTE FUNCTION plux_bump_permissions_version();

-- plux_app_restricted reports whether the signed-in person (plux.user_id)
-- is barred from an app by a restriction that removes every permission:
-- the pattern '*', not limited to an environment, naming the app or no app,
-- whose subject is the organisation, the person or a team they belong to.
-- Rows that belong to no app (a NULL app_id) are never restricted. It reads
-- restriction_policies, which therefore carries no policy that calls it.
CREATE OR REPLACE FUNCTION plux_app_restricted(target uuid) RETURNS boolean
    LANGUAGE sql STABLE PARALLEL SAFE
    AS $$
        SELECT target IS NOT NULL AND plux_current_user() IS NOT NULL AND EXISTS (
            SELECT 1
              FROM restriction_policies r
             WHERE r.organization_id = plux_current_organization()
               AND '*' = ANY (r.patterns)
               AND r.environment IS NULL
               AND (r.app_id IS NULL OR r.app_id = target)
               AND (r.subject_kind = 'organization'
                    OR r.user_id = plux_current_user()
                    OR r.team_id IN (SELECT m.team_id
                                       FROM memberships m
                                      WHERE m.user_id = plux_current_user()
                                        AND m.organization_id = r.organization_id
                                        AND m.team_id IS NOT NULL))
        )
    $$;

COMMENT ON FUNCTION plux_app_restricted(uuid) IS
    'True when a restriction on every permission bars the signed-in person from the app (GOV-006, SEC-102).';

-- plux_app_policy adds the restrictive policy that keeps a barred person
-- out of a table's rows for an app, for reading and for writing. Every
-- migration that creates a row-level-security table with an app_id column
-- calls it; a test walks the catalogue and fails when one does not.
CREATE OR REPLACE FUNCTION plux_app_policy(target regclass, app_column name DEFAULT 'app_id') RETURNS void
    LANGUAGE plpgsql
    AS $$
    BEGIN
        EXECUTE format(
            'CREATE POLICY plux_app_unrestricted ON %s AS RESTRICTIVE USING (NOT plux_app_restricted(%I))',
            target, app_column);
    END;
    $$;

COMMENT ON FUNCTION plux_app_policy(regclass, name) IS
    'Binds a tenant table to plux_app_restricted() through a restrictive policy (GOV-006).';

-- The existing tables: every one with row-level security and an app_id
-- column, except the table the function itself reads, and the apps table,
-- whose own rows are the app.
DO $$
DECLARE
    t regclass;
BEGIN
    FOR t IN
        SELECT c.oid::regclass
          FROM pg_class c
          JOIN pg_namespace n ON n.oid = c.relnamespace
          JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'app_id' AND NOT a.attisdropped
         WHERE c.relkind = 'r'
           AND c.relrowsecurity
           AND n.nspname = current_schema()
           AND c.relname <> 'restriction_policies'
         ORDER BY c.relname
    LOOP
        PERFORM plux_app_policy(t);
    END LOOP;
END;
$$;

SELECT plux_app_policy('apps', 'id');
