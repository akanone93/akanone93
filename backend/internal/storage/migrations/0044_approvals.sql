-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- The approval engine (GOV-020 to GOV-023, ADR-0076).
--
-- approval_policies holds the policy documents of an app, versioned: every
-- change is a new row with the next version, and a deleted policy is a row
-- with deleted set, so that a default policy an administrator removed is
-- not seeded again. The newest row of a key is the policy in force.
--
-- app_id is the app the policy governs, or the reserved UUID
-- 00000000-0000-7000-8000-000000000000 for a policy of the whole
-- organisation, which governs every app and the changes that belong to no
-- app, such as raising an organisation's limit (LIM-006). It is not a
-- reference because that UUID is no app.
CREATE TABLE approval_policies (
    id               uuid        PRIMARY KEY,
    organization_id  uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id           uuid        NOT NULL,
    key              text        NOT NULL,
    version          integer     NOT NULL,
    document         jsonb       NOT NULL,
    document_hash    text        NOT NULL,
    deleted          boolean     NOT NULL DEFAULT false,
    created_by_kind  text        NOT NULL,
    created_by_id    text        NOT NULL,
    created_by       text        NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, app_id, key, version),
    CONSTRAINT approval_policies_key_form CHECK (key ~ '^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$'),
    CONSTRAINT approval_policies_version CHECK (version >= 1)
);
SELECT plux_tenant_policy('approval_policies');

-- approval_requests names one artifact by its content hash (GOV-022). The
-- app and environment are plain identifiers (the reserved UUID above for a
-- change that belongs to no app), not references: a request is
-- opened from a separate transaction while the refused change's own
-- transaction still holds locks on those rows, and a reference would make
-- the two wait for each other. The status is open, approved, rejected,
-- superseded (the subject changed and a newer request replaced this one)
-- or used (an approved request was spent by the change it approved);
-- expiry is read from expires_at.
CREATE TABLE approval_requests (
    id                uuid        PRIMARY KEY,
    organization_id   uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id            uuid        NOT NULL,
    environment_id    uuid,
    environment_key   text        NOT NULL DEFAULT '',
    production        boolean     NOT NULL DEFAULT false,
    resource          text        NOT NULL,
    action            text        NOT NULL,
    subject           text        NOT NULL,
    content_hash      text        NOT NULL,
    summary           text        NOT NULL DEFAULT '',
    risk              text[]      NOT NULL DEFAULT '{}',
    payload           jsonb,
    policy_keys       text[]      NOT NULL DEFAULT '{}',
    approvals_required integer    NOT NULL DEFAULT 1,
    author_id         text        NOT NULL DEFAULT '',
    requested_by_kind text        NOT NULL,
    requested_by_id   text        NOT NULL,
    requested_by      text        NOT NULL,
    status            text        NOT NULL DEFAULT 'open',
    created_at        timestamptz NOT NULL DEFAULT now(),
    expires_at        timestamptz NOT NULL,
    used_at           timestamptz,
    used_by           text        NOT NULL DEFAULT '',
    CONSTRAINT approval_requests_status CHECK (status IN ('open', 'approved', 'rejected', 'superseded', 'used'))
);
CREATE UNIQUE INDEX approval_requests_live ON approval_requests (organization_id, app_id, resource, action, subject, content_hash)
    WHERE status IN ('open', 'approved');
CREATE INDEX approval_requests_app ON approval_requests (app_id, created_at DESC, id DESC);
SELECT plux_tenant_policy('approval_requests');

-- approval_decisions holds each decision as a record: payload is the
-- canonical encoding of what the decider confirmed, and the worker, which
-- alone holds the audit key, signs it later (signature stays NULL until it
-- has). audit_sequence is the entry of the audit chain that records the
-- decision, so the record is covered by the chain and its checkpoints from
-- the moment it is written.
CREATE TABLE approval_decisions (
    id              uuid        PRIMARY KEY,
    request_id      uuid        NOT NULL REFERENCES approval_requests (id) ON DELETE CASCADE,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    decider_id      text        NOT NULL,
    decider         text        NOT NULL,
    roles           text[]      NOT NULL DEFAULT '{}',
    teams           text[]      NOT NULL DEFAULT '{}',
    decision       text        NOT NULL,
    comment         text        NOT NULL DEFAULT '',
    content_hash    text        NOT NULL,
    decided_at      timestamptz NOT NULL,
    audit_sequence  bigint      NOT NULL,
    payload         bytea       NOT NULL,
    key_id          text,
    signature       bytea,
    UNIQUE (request_id, decider_id),
    CONSTRAINT approval_decisions_decision CHECK (decision IN ('approve', 'reject')),
    CONSTRAINT approval_decisions_signed CHECK ((key_id IS NULL) = (signature IS NULL))
);
CREATE INDEX approval_decisions_unsigned ON approval_decisions (decided_at, id) WHERE signature IS NULL;
SELECT plux_tenant_policy('approval_decisions');
