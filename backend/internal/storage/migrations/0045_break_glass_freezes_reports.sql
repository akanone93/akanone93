-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Break-glass, change freezes and compliance reports (GOV-024 to GOV-026,
-- ADR-0076). Expand-only: four new tables.
--
-- The app, environment and request identifiers of the tables that outlive
-- what they name are plain values, not references: a compliance report must
-- still be able to list a change after its app was deleted.

-- break_glass_invocations records each emergency action taken without prior
-- approval (GOV-024): who, what, the written justification, and the
-- post-hoc review that is due. A review is open until someone other than the
-- invoker completes it; escalated_at is set once, when the due time passes
-- with the review still open.
CREATE TABLE break_glass_invocations (
    id               uuid        PRIMARY KEY,
    organization_id  uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id           uuid        NOT NULL,
    environment_id   uuid        NOT NULL,
    environment_key  text        NOT NULL,
    production       boolean     NOT NULL,
    action           text        NOT NULL,
    subject          text        NOT NULL,
    justification    text        NOT NULL,
    invoked_by_kind  text        NOT NULL,
    invoked_by_id    text        NOT NULL,
    invoked_by       text        NOT NULL,
    invoked_at       timestamptz NOT NULL,
    review_due_at    timestamptz NOT NULL,
    review_status    text        NOT NULL DEFAULT 'open',
    reviewed_by_id   text        NOT NULL DEFAULT '',
    reviewed_by      text        NOT NULL DEFAULT '',
    reviewed_at      timestamptz,
    review_comment   text        NOT NULL DEFAULT '',
    escalated_at     timestamptz,
    CONSTRAINT break_glass_action CHECK (action IN ('killSwitch', 'forcedRollback', 'rollback')),
    CONSTRAINT break_glass_review_status CHECK (review_status IN ('open', 'reviewed')),
    CONSTRAINT break_glass_justified CHECK (btrim(justification) <> ''),
    CONSTRAINT break_glass_reviewed CHECK ((review_status = 'reviewed') = (reviewed_at IS NOT NULL))
);
CREATE INDEX break_glass_open ON break_glass_invocations (review_due_at, id) WHERE review_status = 'open';
CREATE INDEX break_glass_by_org ON break_glass_invocations (organization_id, invoked_at DESC, id DESC);
SELECT plux_tenant_policy('break_glass_invocations');

-- change_freezes holds the change-freeze windows of an organisation
-- (GOV-025). start_local is a wall-clock time in time_zone, so that a window
-- follows daylight-saving time; the licence's calendar evaluates it. An empty
-- environment_key freezes every production environment.
CREATE TABLE change_freezes (
    id               uuid        PRIMARY KEY,
    organization_id  uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name             text        NOT NULL,
    environment_key  text        NOT NULL DEFAULT '',
    time_zone        text        NOT NULL,
    start_local      text        NOT NULL,
    duration_minutes integer     NOT NULL,
    recurrence       text        NOT NULL DEFAULT '',
    reason           text        NOT NULL DEFAULT '',
    created_by       text        NOT NULL,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL,
    UNIQUE (organization_id, name),
    CONSTRAINT change_freezes_duration CHECK (duration_minutes >= 1)
);
SELECT plux_tenant_policy('change_freezes');

-- production_changes is the ledger of every change made to a production
-- environment through the approval engine or break-glass (GOV-026): who made
-- it, when, to what content, and the approval request that authorised it.
-- A compliance report is a reading of this table and of the decisions it
-- points to. A row is written in the transaction of the change, so a change
-- that is refused or rolled back leaves none.
CREATE TABLE production_changes (
    id               uuid        PRIMARY KEY,
    organization_id  uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id           uuid        NOT NULL,
    environment_id   uuid,
    environment_key  text        NOT NULL DEFAULT '',
    resource         text        NOT NULL,
    action           text        NOT NULL,
    subject          text        NOT NULL,
    content_hash     text        NOT NULL,
    summary          text        NOT NULL DEFAULT '',
    author_id        text        NOT NULL DEFAULT '',
    changed_by_kind  text        NOT NULL,
    changed_by_id    text        NOT NULL,
    changed_by       text        NOT NULL,
    changed_at       timestamptz NOT NULL,
    request_id       uuid,
    break_glass_id   uuid
);
CREATE INDEX production_changes_period ON production_changes (organization_id, changed_at, id);
SELECT plux_tenant_policy('production_changes');

-- compliance_reports holds a requested report: the statement is built when
-- the report is requested, and the worker, which alone holds the audit key,
-- signs it later (signature stays NULL until it has), as it does for the
-- decisions of the approval engine.
CREATE TABLE compliance_reports (
    id               uuid        PRIMARY KEY,
    organization_id  uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    period_from      timestamptz NOT NULL,
    period_to        timestamptz NOT NULL,
    changes          integer     NOT NULL,
    requested_by_id  text        NOT NULL,
    requested_by     text        NOT NULL,
    requested_at     timestamptz NOT NULL,
    statement        bytea       NOT NULL,
    key_id           text,
    signature        bytea,
    signed_at        timestamptz,
    CONSTRAINT compliance_reports_period CHECK (period_from < period_to),
    CONSTRAINT compliance_reports_signed CHECK ((key_id IS NULL) = (signature IS NULL) AND (key_id IS NULL) = (signed_at IS NULL))
);
CREATE INDEX compliance_reports_by_org ON compliance_reports (organization_id, requested_at DESC, id DESC);
CREATE INDEX compliance_reports_unsigned ON compliance_reports (requested_at, id) WHERE signature IS NULL;
SELECT plux_tenant_policy('compliance_reports');
