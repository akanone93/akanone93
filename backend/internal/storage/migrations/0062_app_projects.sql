-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Shell settings and host-project generation jobs (P11 S8.1, plan §4.3 #10,
-- GEN-004, F18). Expand-only: new tables. app_shell_settings keeps what the
-- app's shell is configured as (settings) and what its project was last
-- generated from (generated), so that "shell update required" is a
-- comparison of two documents. A generation job is a row the worker claims;
-- at most one is active per app. The zip is kept apart so that a status poll
-- never reads it.
CREATE TABLE app_shell_settings (
    app_id          uuid        PRIMARY KEY REFERENCES apps (id) ON DELETE CASCADE,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    settings        jsonb       NOT NULL,
    generated       jsonb,
    generated_at    timestamptz,
    updated_at      timestamptz NOT NULL DEFAULT now()
);
SELECT plux_tenant_policy('app_shell_settings');
SELECT plux_app_policy('app_shell_settings');

CREATE TABLE project_jobs (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_key text        NOT NULL,
    channel_key     text        NOT NULL,
    format          text        NOT NULL,
    status          text        NOT NULL DEFAULT 'queued',
    error_code      text        NOT NULL DEFAULT '',
    error_message   text        NOT NULL DEFAULT '',
    size            bigint      NOT NULL DEFAULT 0,
    sha256          text        NOT NULL DEFAULT '',
    created_by      text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    finished_at     timestamptz,
    CONSTRAINT project_jobs_format CHECK (format IN ('zip')),
    CONSTRAINT project_jobs_status CHECK (status IN ('queued', 'running', 'succeeded', 'failed'))
);
CREATE UNIQUE INDEX project_jobs_active ON project_jobs (app_id) WHERE status IN ('queued', 'running');
CREATE INDEX project_jobs_app ON project_jobs (app_id, created_at DESC, id DESC);
SELECT plux_tenant_policy('project_jobs');
SELECT plux_app_policy('project_jobs');

CREATE TABLE project_job_results (
    job_id          uuid        PRIMARY KEY REFERENCES project_jobs (id) ON DELETE CASCADE,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    data            bytea       NOT NULL
);
SELECT plux_tenant_policy('project_job_results');
SELECT plux_app_policy('project_job_results');
