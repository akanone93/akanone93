-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- A developer's phone signs in to the Plux Dev app once and then joins that
-- developer's own Studio dev sessions (DEV-002, ADR-0104). Expand-only: rows
-- and code of the previous version stay valid.

-- A phone's sign-in is a token of its own source, bound to the thumbprint of
-- the phone's DPoP key: it is accepted only with a proof of that key, and only
-- by the procedures of a signed-in phone. platform describes the phone.
ALTER TABLE access_tokens DROP CONSTRAINT access_tokens_source;
ALTER TABLE access_tokens ADD CONSTRAINT access_tokens_source CHECK (source IN ('pat', 'cli', 'ci', 'seed', 'phone'));
ALTER TABLE access_tokens
    ADD COLUMN dpop_thumbprint text NOT NULL DEFAULT '',
    ADD COLUMN platform        text NOT NULL DEFAULT '';
ALTER TABLE access_tokens ADD CONSTRAINT access_tokens_phone_key CHECK ((source = 'phone') = (dpop_thumbprint <> ''));

-- The QR code Studio shows to connect a phone. It holds a grant, stored only
-- as its hash, that a phone claims once with its DPoP key ('claimed': Studio
-- now shows number); picking that number among three on the phone signs it
-- in ('signed_in'), a wrong pick ends it ('failed'). Found by the grant's hash
-- before its organisation is known, so the authentication scope may read it,
-- as for access_tokens.
CREATE TABLE dev_phone_sign_ins (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    grant_hash      bytea       NOT NULL UNIQUE,
    state           text        NOT NULL DEFAULT 'waiting',
    dpop_thumbprint text        NOT NULL DEFAULT '',
    number          smallint    NOT NULL DEFAULT 0,
    phone_name      text        NOT NULL DEFAULT '',
    platform        text        NOT NULL DEFAULT '',
    token_id        uuid        REFERENCES access_tokens (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    claimed_at      timestamptz,
    expires_at      timestamptz NOT NULL,
    CONSTRAINT dev_phone_sign_ins_state CHECK (state IN ('waiting', 'claimed', 'signed_in', 'failed', 'cancelled')),
    CONSTRAINT dev_phone_sign_ins_hash CHECK (length(grant_hash) = 32),
    CONSTRAINT dev_phone_sign_ins_number CHECK (number = 0 OR number BETWEEN 10 AND 99)
);
CREATE INDEX dev_phone_sign_ins_org ON dev_phone_sign_ins (organization_id);
CREATE INDEX dev_phone_sign_ins_key ON dev_phone_sign_ins (dpop_thumbprint, claimed_at);
ALTER TABLE dev_phone_sign_ins ENABLE ROW LEVEL SECURITY;
ALTER TABLE dev_phone_sign_ins FORCE ROW LEVEL SECURITY;
CREATE POLICY plux_tenant ON dev_phone_sign_ins
    USING (organization_id = plux_current_organization() OR plux_in_scope('authentication'))
    WITH CHECK (organization_id = plux_current_organization() OR plux_in_scope('authentication'));

-- A signed-in phone's request to join a Studio dev session of its own
-- person. Studio shows number to that person; picking it on the phone joins
-- ('joined') and lets the phone join the same session again without a number
-- while the session is live; a refusal in Studio ('denied') ends the request.
-- A wrong pick ('failed') ends it and keeps the phone out of the session until
-- the person lets it try again in Studio ('retry'), so that a phone in other
-- hands gets one guess, not one per request.
CREATE TABLE dev_join_requests (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL,
    session_id      uuid        NOT NULL REFERENCES dev_sessions (id) ON DELETE CASCADE,
    token_id        uuid        NOT NULL REFERENCES access_tokens (id) ON DELETE CASCADE,
    state           text        NOT NULL DEFAULT 'waiting',
    number          smallint    NOT NULL,
    phone_name      text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    CONSTRAINT dev_join_requests_state CHECK (state IN ('waiting', 'joined', 'failed', 'denied', 'retry')),
    CONSTRAINT dev_join_requests_number CHECK (number BETWEEN 10 AND 99)
);
CREATE INDEX dev_join_requests_session ON dev_join_requests (session_id, token_id);
SELECT plux_tenant_policy('dev_join_requests');
SELECT plux_app_policy('dev_join_requests');
