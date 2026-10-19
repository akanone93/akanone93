-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Audit export to a SIEM (SEC-143, ADR-0082): where an organisation's audit
-- chain is delivered and how far delivery has got. A destination and its
-- cursor are one row, so advancing the cursor is a single statement in the
-- transaction that holds the row. The table is new and nothing existing is
-- altered (SRV-021).
CREATE TABLE audit_export_destinations (
    id                      uuid        PRIMARY KEY,
    organization_id         uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name                    text        NOT NULL,
    -- kind is 'syslog' in the core; 'cef', 'otlp' and 'webhook' are
    -- enterprise sinks (GOV-030).
    kind                    text        NOT NULL,
    -- address is host:port for the syslog kinds and a URL for the HTTP kinds.
    address                 text        NOT NULL,
    enabled                 boolean     NOT NULL DEFAULT true,
    -- allow_private lets the destination be a private or loopback address;
    -- it exists for development and is reported with a warning (SEC-104).
    allow_private           boolean     NOT NULL DEFAULT false,
    -- ca_pem pins the roots that verify the destination's certificate; empty
    -- means the system roots.
    ca_pem                  text        NOT NULL DEFAULT '',
    -- The client certificate and key, or the webhook signing secret, sealed
    -- with envelope encryption (SEC-106); empty when there are none.
    credentials_ciphertext  bytea       NOT NULL DEFAULT '',
    credentials_wrapped_key bytea       NOT NULL DEFAULT '',
    credentials_key_id      text        NOT NULL DEFAULT '',
    credentials_hint        text        NOT NULL DEFAULT '',
    -- The cursor: the last sequence of the organisation's chain the sink
    -- acknowledged, and that entry's hash. It moves only after an
    -- acknowledgement, so delivery is at least once and in order.
    cursor_sequence         bigint      NOT NULL DEFAULT 0,
    cursor_hash             text        NOT NULL DEFAULT '',
    exported_count          bigint      NOT NULL DEFAULT 0,
    -- Failure bookkeeping: consecutive failures, when the next attempt may
    -- start (exponential backoff with jitter) and the last error, which never
    -- carries a credential.
    failures                integer     NOT NULL DEFAULT 0,
    next_attempt_at         timestamptz NOT NULL DEFAULT now(),
    last_error              text        NOT NULL DEFAULT '',
    last_error_at           timestamptz,
    last_success_at         timestamptz,
    version                 bigint      NOT NULL DEFAULT 1,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name),
    CONSTRAINT audit_export_destinations_name CHECK (name ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$' AND length(name) <= 64),
    CONSTRAINT audit_export_destinations_kind CHECK (kind IN ('syslog', 'cef', 'otlp', 'webhook')),
    CONSTRAINT audit_export_destinations_cursor CHECK (cursor_sequence >= 0 AND exported_count >= 0 AND failures >= 0)
);
CREATE INDEX audit_export_destinations_due ON audit_export_destinations (organization_id, next_attempt_at) WHERE enabled;
SELECT plux_tenant_policy('audit_export_destinations');
