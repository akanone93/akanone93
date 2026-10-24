-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Dev sessions across api replicas (P11 S0.9, DEV-021, ADR-0087 Revision
-- 2026-10-24). Expand-only: two columns with defaults and a new table.
--
-- kind says who drives a session: Studio sessions follow the stored draft,
-- which the server compiles and pushes after each accepted write (contract
-- #17); CLI sessions push patches over the stored draft. Sessions made
-- before this migration were made by the CLI.
ALTER TABLE dev_sessions ADD COLUMN kind text NOT NULL DEFAULT 'cli';
ALTER TABLE dev_sessions
    ADD CONSTRAINT dev_sessions_kind CHECK (kind IN ('studio', 'cli'));
-- bundle_sequence is the number of the last development bundle pushed to the
-- session, counted in the database so that every replica continues it.
ALTER TABLE dev_sessions ADD COLUMN bundle_sequence bigint NOT NULL DEFAULT 0;
CREATE INDEX dev_sessions_following ON dev_sessions (app_id, kind) WHERE ended_at IS NULL;

-- dev_session_patches is the overlay of a CLI session: the patches it pushed,
-- in order, one row per document patch. A replica rebuilds a session's draft
-- by replaying them over the stored draft. The bytes of a session's patches
-- are bounded by devSession.patchLogBytes; the log goes with its session.
CREATE TABLE dev_session_patches (
    session_id      uuid        NOT NULL REFERENCES dev_sessions (id) ON DELETE CASCADE,
    seq             bigint      NOT NULL,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    path            text        NOT NULL,
    patch           jsonb       NOT NULL,
    bytes           integer     NOT NULL,
    created_at      timestamptz NOT NULL,
    PRIMARY KEY (session_id, seq),
    CONSTRAINT dev_session_patches_seq CHECK (seq > 0),
    CONSTRAINT dev_session_patches_bytes CHECK (bytes >= 0)
);
SELECT plux_tenant_policy('dev_session_patches');
