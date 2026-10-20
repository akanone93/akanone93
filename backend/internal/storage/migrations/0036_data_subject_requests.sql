-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Data subject requests (SEC-162). Every change here adds a table: nothing
-- existing is altered (DEP-030). Both tables carry their organisation and
-- are bound to it by row-level security (SRV-022).
--
-- A subject is named only by its hash: sha256 over the organisation, the
-- kind and the identifier. The identifier itself is never stored here, so a
-- tombstone cannot bring the erased subject back.

-- One completed request: an export or an erasure of a device or a
-- pseudonymous user. subject_hash is the same value the tombstones carry.
CREATE TABLE dsr_requests (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    action          text        NOT NULL,
    subject_kind    text        NOT NULL,
    subject_hash    bytea       NOT NULL,
    actor           text        NOT NULL,
    row_count       bigint      NOT NULL DEFAULT 0,
    archive_bytes   bigint      NOT NULL DEFAULT 0,
    requested_at    timestamptz NOT NULL,
    CONSTRAINT dsr_requests_action CHECK (action IN ('export', 'erase')),
    CONSTRAINT dsr_requests_kind CHECK (subject_kind IN ('device', 'user')),
    CONSTRAINT dsr_requests_counts CHECK (row_count >= 0 AND archive_bytes >= 0)
);
CREATE INDEX dsr_requests_org ON dsr_requests (organization_id, requested_at DESC, id DESC);
SELECT plux_tenant_policy('dsr_requests');

-- What an erasure left behind: the subject's hash, the store, how many rows
-- went and when. A store the subject had nothing in leaves no tombstone.
CREATE TABLE dsr_tombstones (
    id              bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    request_id      uuid        NOT NULL,
    subject_kind    text        NOT NULL,
    subject_hash    bytea       NOT NULL,
    store           text        NOT NULL,
    row_count       bigint      NOT NULL,
    erased_at       timestamptz NOT NULL,
    CONSTRAINT dsr_tombstones_kind CHECK (subject_kind IN ('device', 'user')),
    CONSTRAINT dsr_tombstones_count CHECK (row_count > 0)
);
CREATE INDEX dsr_tombstones_subject ON dsr_tombstones (organization_id, subject_hash);
CREATE INDEX dsr_tombstones_request ON dsr_tombstones (request_id);
SELECT plux_tenant_policy('dsr_tombstones');
