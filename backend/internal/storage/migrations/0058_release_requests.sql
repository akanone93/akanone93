-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Release requests and visual-diff render jobs (STU-100, ADR-0097, P11 S7.1).
-- The tables are new and nothing existing is altered, so a replica of the
-- previous version keeps working during a rolling upgrade (SRV-021).

-- A release request is the server-side state of Studio's publish wizard
-- (F11). state is the step the request is at, or how it ended; hold says why
-- a request waits at its step. content_hash is the SHA-256 of what the
-- builds produced; an approval is bound to it (GOV-022): approved_hash is the
-- content hash an approval was given for, and a request whose content hash
-- moves away from it loses the approval.
CREATE TABLE release_requests (
    id                  uuid        PRIMARY KEY,
    organization_id     uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id              uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id      uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    channel             text        NOT NULL,
    state               text        NOT NULL,
    hold                text        NOT NULL DEFAULT '',
    hold_reason         text        NOT NULL DEFAULT '',
    revision            bigint      NOT NULL DEFAULT 1,
    plugins             jsonb       NOT NULL DEFAULT '[]',
    builds              jsonb       NOT NULL DEFAULT '[]',
    acknowledged        text[]      NOT NULL DEFAULT '{}',
    content_hash        text        NOT NULL DEFAULT '',
    changelog_notes     text        NOT NULL DEFAULT '',
    label               text        NOT NULL DEFAULT '',
    rollout_plan        jsonb,
    approval_request_id text        NOT NULL DEFAULT '',
    approved_hash       text        NOT NULL DEFAULT '',
    release_sequence    bigint,
    created_by          text        NOT NULL,
    created_by_name     text        NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    ended_at            timestamptz,
    CONSTRAINT release_requests_state CHECK (state IN (
        'select', 'build', 'reports', 'diffs', 'changelog', 'target', 'approval', 'rollout', 'release',
        'released', 'rejected', 'expired', 'cancelled')),
    CONSTRAINT release_requests_hold CHECK (hold IN ('', 'failed', 'blocked', 'awaiting_approval')),
    CONSTRAINT release_requests_channel_form CHECK (channel ~ '^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$')
);
-- At most one open request per app, environment and channel; a second
-- attempt finds the open one (F11).
CREATE UNIQUE INDEX release_requests_one_open
    ON release_requests (app_id, environment_id, channel)
    WHERE state NOT IN ('released', 'rejected', 'expired', 'cancelled');
CREATE INDEX release_requests_app ON release_requests (app_id, created_at DESC, id DESC);
SELECT plux_tenant_policy('release_requests');

-- A visual-diff render job (O-9): queued, run by a render worker that holds no
-- database credentials, rate-limited per organisation by
-- render.visualDiffJobsPerOrganization. result holds the screens rendered on
-- the standard devices; the images are in object storage by SHA-256.
CREATE TABLE visual_diff_jobs (
    id                 uuid        PRIMARY KEY,
    organization_id    uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    release_request_id uuid        NOT NULL REFERENCES release_requests (id) ON DELETE CASCADE,
    content_hash       text        NOT NULL,
    state              text        NOT NULL DEFAULT 'queued',
    result             jsonb,
    error              text        NOT NULL DEFAULT '',
    attempts           integer     NOT NULL DEFAULT 0,
    created_by         text        NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    started_at         timestamptz,
    finished_at        timestamptz,
    CONSTRAINT visual_diff_jobs_state CHECK (state IN ('queued', 'running', 'succeeded', 'failed'))
);
-- One job in flight per request and content; asking again returns it.
CREATE UNIQUE INDEX visual_diff_jobs_in_flight
    ON visual_diff_jobs (release_request_id, content_hash)
    WHERE state IN ('queued', 'running');
CREATE INDEX visual_diff_jobs_org_running ON visual_diff_jobs (organization_id) WHERE state = 'running';
CREATE INDEX visual_diff_jobs_request ON visual_diff_jobs (release_request_id, created_at DESC);
SELECT plux_tenant_policy('visual_diff_jobs');
