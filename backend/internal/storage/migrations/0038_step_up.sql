-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- WebAuthn step-up (SEC-103, ADR-0075). A request is one ceremony. For a
-- browser session ('session') the assertion sets sessions.step_up_at. For
-- the CLI ('cli', maintainer decision Q15) the browser runs the ceremony and
-- the server issues a grant bound to the CLI token and to the thumbprint of
-- the DPoP key the CLI made the request with; the CLI polls for it with a
-- proof of the same key. Only hashes of the request and grant secrets are
-- stored.
CREATE TABLE step_up_requests (
    id               uuid        PRIMARY KEY,
    user_id          uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- organization_id is the organisation of the CLI token a request was
    -- made with, whose audit log records it; a browser session has none.
    organization_id  uuid        REFERENCES organizations (id) ON DELETE CASCADE,
    kind             text        NOT NULL,
    session_id       uuid        REFERENCES sessions (id) ON DELETE CASCADE,
    -- token_id is the CLI token the request was made with; the grant is
    -- usable only with that token.
    token_id         uuid        REFERENCES access_tokens (id) ON DELETE CASCADE,
    -- jkt is the RFC 7638 thumbprint of the CLI's DPoP key.
    jkt              text        NOT NULL DEFAULT '',
    -- secret_hash is the hash of the one-time secret the CLI polls with.
    secret_hash      bytea       UNIQUE,
    -- challenge is the WebAuthn challenge of the open ceremony; it is
    -- cleared as soon as an assertion is presented, right or wrong.
    challenge        bytea,
    -- attempts counts the ceremonies begun for the request.
    attempts         integer     NOT NULL DEFAULT 0,
    -- state is 'pending', 'approved' (the assertion verified), 'issued'
    -- (the grant was handed to the CLI) or 'closed'.
    state            text        NOT NULL DEFAULT 'pending',
    -- approved_at is when the assertion verified; a grant is as old as it.
    approved_at      timestamptz,
    grant_hash       bytea       UNIQUE,
    grant_expires_at timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    expires_at       timestamptz NOT NULL,
    CONSTRAINT step_up_kind  CHECK (kind IN ('session', 'cli')),
    CONSTRAINT step_up_state CHECK (state IN ('pending', 'approved', 'issued', 'closed')),
    CONSTRAINT step_up_owner CHECK (
        (kind = 'session' AND session_id IS NOT NULL AND token_id IS NULL AND organization_id IS NULL AND jkt = '' AND secret_hash IS NULL)
        OR (kind = 'cli' AND token_id IS NOT NULL AND session_id IS NULL AND organization_id IS NOT NULL AND jkt <> '' AND secret_hash IS NOT NULL))
);

CREATE INDEX step_up_requests_expiry ON step_up_requests (expires_at);

-- A request of the CLI's belongs to the organisation of its token and is
-- seen inside it; a browser session's belongs to none and is seen only in
-- the installation scope. The authentication scope finds a request before
-- its organisation is known, which the page a person opens and the
-- maintenance sweep need.
ALTER TABLE step_up_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE step_up_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY plux_tenant ON step_up_requests
    USING (organization_id = plux_current_organization()
        OR (organization_id IS NULL AND plux_in_scope('installation'))
        OR plux_in_scope('authentication'))
    WITH CHECK (organization_id = plux_current_organization()
        OR (organization_id IS NULL AND plux_in_scope('installation'))
        OR plux_in_scope('authentication'));
