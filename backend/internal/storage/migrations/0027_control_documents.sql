-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- The signed control document of a channel (SYN-060, REL-060, REL-070,
-- ADR-0077). Every change of a channel's switches is a new row with the
-- next version, so the table is the history of the channel's control. The
-- api role writes the content; the worker, which alone holds the control
-- key, signs it (SRV-052): signed stays NULL until it has. The newest
-- signed row is what devices are served.
--
-- kill_switches is a JSON array of {"scope", "target"}, sorted by scope
-- and target; mandatory_update is {"releaseId", "releaseSequence",
-- "messageKey"} and forced_rollback {"releaseId", "releaseSequence"}, both
-- NULL when not set. The table is new; channel_controls stays as it is for
-- one release, the source of the manifest's control flags (REL-030).
CREATE TABLE control_documents (
    channel_id       uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    organization_id  uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    version          bigint      NOT NULL,
    kill_switches    jsonb       NOT NULL DEFAULT '[]',
    mandatory_update jsonb,
    forced_rollback  jsonb,
    signed           bytea,
    signed_sha256    bytea,
    issued_at        timestamptz,
    expires_at       timestamptz,
    created_by_kind  text        NOT NULL,
    created_by_id    text        NOT NULL,
    created_by       text        NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (channel_id, version),
    CONSTRAINT control_documents_version CHECK (version >= 1),
    CONSTRAINT control_documents_signed CHECK (
        (signed IS NULL AND signed_sha256 IS NULL AND issued_at IS NULL AND expires_at IS NULL)
        OR (signed IS NOT NULL AND signed_sha256 IS NOT NULL AND issued_at IS NOT NULL AND expires_at IS NOT NULL))
);
CREATE INDEX control_documents_signed_newest ON control_documents (channel_id, version DESC) WHERE signed IS NOT NULL;
SELECT plux_tenant_policy('control_documents');

-- The control role signs with an online key like the others, so its public
-- key is recorded for GetRootKeys.
ALTER TABLE environment_keys DROP CONSTRAINT environment_keys_role;
ALTER TABLE environment_keys ADD CONSTRAINT environment_keys_role
    CHECK (role IN ('targets', 'snapshot', 'timestamp', 'control'));

-- Every channel that has switches today gets them as version 1 of its
-- control document, unsigned: the worker signs it at its next sweep. The
-- plugin kill switches become plugin-scope kill switches. A mandatory
-- update flag becomes a mandatory update of the channel's current release,
-- when it has one. The app-wide kill switch and the message have no place
-- in the new document and stay in channel_controls, which the manifest
-- still reads for one release. Row-level security binds the migration like
-- every other session, so each organisation is migrated under its own
-- identity.
DO $$
DECLARE
    org uuid;
BEGIN
    FOR org IN SELECT id FROM organizations ORDER BY id LOOP
        PERFORM set_config('plux.organization_id', org::text, true);
        INSERT INTO control_documents (channel_id, organization_id, version, kill_switches, mandatory_update,
                                       created_by_kind, created_by_id, created_by, created_at)
        SELECT c.channel_id, c.organization_id, 1,
               COALESCE((SELECT jsonb_agg(jsonb_build_object('scope', 'plugin', 'target', k) ORDER BY k)
                           FROM (SELECT DISTINCT unnest(c.kill_switch_plugins) AS k) AS plugins), '[]'::jsonb),
               (SELECT jsonb_build_object('releaseId', r.id::text, 'releaseSequence', r.sequence, 'messageKey', '')
                  FROM channels ch
                  JOIN environments e ON e.id = ch.environment_id
                  JOIN releases r ON r.app_id = e.app_id AND r.sequence = ch.release_sequence
                 WHERE ch.id = c.channel_id AND c.mandatory_update),
               c.updated_by_kind, c.updated_by_id, c.updated_by, c.updated_at
          FROM channel_controls c
         WHERE c.organization_id = org
            AND (cardinality(c.kill_switch_plugins) > 0 OR c.mandatory_update);
    END LOOP;
    PERFORM set_config('plux.organization_id', '', true);
END
$$;
