-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Feature flags and experiments of an environment (ABT-006, ABT-001,
-- ABT-004, ADR-0079). Every change here adds a table: nothing existing is
-- altered. All tables carry their organisation and are bound to it by
-- row-level security (SRV-022).
--
-- What a device reads is the control document (control_documents); these
-- tables are the administered state the server builds it from.

-- A feature flag of an environment: its type, default and the value per
-- segment. segments is a JSON array of {"segment", "value"} in precedence
-- order; values are JSON. json_schema is the JSON Schema of a json flag's
-- values. A flag's type never changes once it exists (PLX-8203): the flag
-- is deleted and made again instead.
CREATE TABLE feature_flags (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id  uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    key             text        NOT NULL,
    type            text        NOT NULL,
    description     text        NOT NULL DEFAULT '',
    json_schema     jsonb,
    default_value   jsonb       NOT NULL,
    segments        jsonb       NOT NULL DEFAULT '[]',
    created_by_kind text        NOT NULL,
    created_by_id   text        NOT NULL,
    created_by      text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_by_kind text        NOT NULL,
    updated_by_id   text        NOT NULL,
    updated_by      text        NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT feature_flags_key UNIQUE (environment_id, key),
    CONSTRAINT feature_flags_type CHECK (type IN ('bool', 'number', 'string', 'json'))
);
CREATE INDEX feature_flags_app ON feature_flags (app_id, key);
SELECT plux_tenant_policy('feature_flags');

-- An experiment of an environment. definition is the experiment document
-- (schema/json/experiment.schema.json) in canonical JSON, whose key this row
-- repeats. state moves draft, running, stopped, concluded and never back.
-- layer_start and layer_end are the bucket range of the layer the experiment
-- holds while it runs (ABT-004); both are NULL outside a layer or while not
-- running. layer_share_bp is the share of the layer asked for at start.
-- segment_rule is the targeting rule of the experiment's segment as it was
-- resolved when the experiment started, in the form of package controldoc;
-- NULL for an experiment on everyone. winner and promote_job_id are set when
-- the winner was promoted (ABT-005).
CREATE TABLE experiments (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id  uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    key             text        NOT NULL,
    state           text        NOT NULL DEFAULT 'draft',
    definition      jsonb       NOT NULL,
    salt            text        NOT NULL,
    layer           text        NOT NULL DEFAULT '',
    layer_share_bp  integer     NOT NULL DEFAULT 0,
    layer_start     integer,
    layer_end       integer,
    segment_rule    jsonb,
    started_at      timestamptz,
    stopped_at      timestamptz,
    winner          text        NOT NULL DEFAULT '',
    promote_job_id  uuid,
    created_by_kind text        NOT NULL,
    created_by_id   text        NOT NULL,
    created_by      text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT experiments_key UNIQUE (environment_id, key),
    CONSTRAINT experiments_state CHECK (state IN ('draft', 'running', 'stopped', 'concluded')),
    CONSTRAINT experiments_layer_range CHECK (
        (layer_start IS NULL AND layer_end IS NULL)
        OR (layer_start >= 0 AND layer_end > layer_start AND layer_end <= 10000)),
    CONSTRAINT experiments_layer_share CHECK (layer_share_bp BETWEEN 0 AND 10000)
);
CREATE INDEX experiments_app_state ON experiments (app_id, state);
CREATE INDEX experiments_layer ON experiments (environment_id, layer) WHERE state = 'running' AND layer <> '';
SELECT plux_tenant_policy('experiments');

-- The share of an environment's subjects kept out of every experiment
-- (ABT-004). The salt is generated once, so membership is stable.
CREATE TABLE experiment_holdouts (
    environment_id  uuid        PRIMARY KEY REFERENCES environments (id) ON DELETE CASCADE,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    basis_points    integer     NOT NULL,
    salt            text        NOT NULL,
    updated_by_kind text        NOT NULL,
    updated_by_id   text        NOT NULL,
    updated_by      text        NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT experiment_holdouts_share CHECK (basis_points BETWEEN 0 AND 10000)
);
SELECT plux_tenant_policy('experiment_holdouts');
