-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Rollouts (REL-050 to REL-053, ADR-0078): a release offered to a share of
-- a channel's devices, in stages, with a health gate. Everything here is
-- additive (DEP-030): one new table, and a nullable column on manifests that
-- marks the manifests signed for a rollout's candidate release.

-- One rollout. While it is scheduled, active or paused the channel keeps
-- pointing at the release it had; devices inside the rollout are served the
-- candidate's manifest instead. Finishing it either points the channel at
-- the candidate (close_action 'promote') or at a new release that repeats
-- the previous one (close_action 'revert', REL-006), so no device ever
-- moves to a lower release sequence.
CREATE TABLE rollouts (
    id                 uuid        PRIMARY KEY,
    organization_id    uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id             uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id     uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    channel_id         uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    -- The keys of the environment and the channel never change; they are kept
    -- for listing without joins.
    environment_key    text        NOT NULL,
    channel_key        text        NOT NULL,
    candidate_sequence bigint      NOT NULL,
    -- previous_sequence is the channel's release when the rollout starts.
    previous_sequence  bigint,
    -- salt makes the buckets of one rollout independent of another's.
    salt               bytea       NOT NULL,
    state              text        NOT NULL,
    -- close_action and close_state are set while state is 'closing': what the
    -- worker still has to do, and the state the rollout ends in.
    close_action       text,
    close_state        text,
    -- stages is the schedule: a list of {basisPoints, durationSeconds}.
    stages             jsonb       NOT NULL,
    stage_index        integer     NOT NULL DEFAULT 0,
    stage_started_at   timestamptz,
    -- basis_points is the share of buckets, in 1/10000, now served the candidate.
    basis_points       integer     NOT NULL DEFAULT 0,
    start_at           timestamptz NOT NULL,
    end_at             timestamptz,
    -- targeting holds the rule and the named segments' rules.
    targeting          jsonb       NOT NULL DEFAULT '{}',
    allow_devices      uuid[]      NOT NULL DEFAULT '{}',
    deny_devices       uuid[]      NOT NULL DEFAULT '{}',
    -- health holds the gate's thresholds; '{}' has no gate.
    health             jsonb       NOT NULL DEFAULT '{}',
    state_reason       text        NOT NULL DEFAULT '',
    -- last_check is the gate's newest verdict with the figures behind it.
    last_check         jsonb,
    last_check_at      timestamptz,
    reverted_sequence  bigint,
    created_by         text        NOT NULL,
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL,
    CONSTRAINT rollouts_state CHECK (state IN ('scheduled', 'active', 'paused', 'closing', 'completed', 'ended', 'aborted', 'rolled_back')),
    CONSTRAINT rollouts_close CHECK ((state = 'closing') = (close_action IS NOT NULL AND close_state IS NOT NULL)),
    CONSTRAINT rollouts_close_action CHECK (close_action IS NULL OR close_action IN ('promote', 'revert')),
    CONSTRAINT rollouts_close_state CHECK (close_state IS NULL OR close_state IN ('completed', 'ended', 'aborted', 'rolled_back')),
    CONSTRAINT rollouts_basis_points CHECK (basis_points BETWEEN 0 AND 10000),
    CONSTRAINT rollouts_window CHECK (end_at IS NULL OR end_at > start_at),
    CONSTRAINT rollouts_candidate CHECK (candidate_sequence > 0 AND (previous_sequence IS NULL OR previous_sequence > 0))
);
CREATE INDEX rollouts_channel ON rollouts (channel_id, created_at DESC, id DESC);
CREATE INDEX rollouts_app ON rollouts (app_id, created_at DESC, id DESC);
CREATE INDEX rollouts_open ON rollouts (organization_id) WHERE state IN ('scheduled', 'active', 'paused', 'closing');
SELECT plux_tenant_policy('rollouts');

-- A manifest signed for a rollout's candidate release. The channel's own
-- manifests, which every other query reads, have no rollout.
ALTER TABLE manifests ADD COLUMN rollout_id uuid REFERENCES rollouts (id) ON DELETE CASCADE;
CREATE INDEX manifests_rollout ON manifests (rollout_id, issued_at DESC, id DESC) WHERE rollout_id IS NOT NULL;
