-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Notifications, outbound webhooks and function triggers (SRV-062, SRV-063,
-- FN-025, ADR-0081). Every table is new and nothing existing is altered, so
-- a replica of the previous version keeps working during a rolling upgrade
-- (SRV-021). Secrets are sealed with the envelope encryption of SEC-106;
-- the columns hold ciphertext, never the value.

-- A destination for notifications: an SMTP relay, or a Slack, Microsoft
-- Teams or Telegram webhook. settings holds what is not secret; the sealed
-- secret is the SMTP password, the webhook URL or the bot token. A webhook
-- URL is a credential, so it is sealed too.
CREATE TABLE notification_channels (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            text        NOT NULL,
    kind            text        NOT NULL,
    settings        jsonb       NOT NULL DEFAULT '{}',
    ciphertext      bytea       NOT NULL DEFAULT ''::bytea,
    wrapped_key     bytea       NOT NULL DEFAULT ''::bytea,
    key_id          text        NOT NULL DEFAULT '',
    hint            text        NOT NULL DEFAULT '',
    enabled         boolean     NOT NULL DEFAULT true,
    created_by      uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name),
    CONSTRAINT notification_channels_kind CHECK (kind IN ('smtp', 'slack', 'teams', 'telegram')),
    CONSTRAINT notification_channels_name CHECK (length(name) BETWEEN 1 AND 64)
);
SELECT plux_tenant_policy('notification_channels');

-- A user or a team follows an event type, into the in-app inbox (channel_id
-- NULL) or through a channel. The all-zero UUID stands for the inbox in the
-- unique index so that NULLs do not defeat it.
CREATE TABLE notification_subscriptions (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    subject_kind    text        NOT NULL,
    subject_id      uuid        NOT NULL,
    event_type      text        NOT NULL,
    channel_id      uuid        REFERENCES notification_channels (id) ON DELETE CASCADE,
    created_by      uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT notification_subscriptions_subject CHECK (subject_kind IN ('user', 'team'))
);
CREATE UNIQUE INDEX notification_subscriptions_unique ON notification_subscriptions
    (organization_id, subject_kind, subject_id, event_type, COALESCE(channel_id, '00000000-0000-0000-0000-000000000000'::uuid));
CREATE INDEX notification_subscriptions_event ON notification_subscriptions (organization_id, event_type);
SELECT plux_tenant_policy('notification_subscriptions');

-- The in-app inbox.
CREATE TABLE notifications (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    event_id        uuid        NOT NULL,
    event_type      text        NOT NULL,
    app_id          uuid,
    title           text        NOT NULL,
    body            text        NOT NULL,
    link            text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    read_at         timestamptz
);
CREATE INDEX notifications_inbox ON notifications (organization_id, user_id, created_at DESC, id DESC);
SELECT plux_tenant_policy('notifications');

-- An outbound webhook endpoint (SRV-063). event_types is empty for every
-- event.
CREATE TABLE webhook_endpoints (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            text        NOT NULL,
    url             text        NOT NULL,
    event_types     text[]      NOT NULL DEFAULT '{}',
    enabled         boolean     NOT NULL DEFAULT true,
    created_by      uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name),
    CONSTRAINT webhook_endpoints_name CHECK (length(name) BETWEEN 1 AND 64)
);
SELECT plux_tenant_policy('webhook_endpoints');

-- The signing secrets of an outbound endpoint or an inbound hook. The
-- newest has no expiry; during a rotation the previous one expires after
-- the overlap, and every secret not yet expired is valid (SRV-063).
CREATE TABLE webhook_secrets (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    owner_kind      text        NOT NULL,
    owner_id        uuid        NOT NULL,
    ciphertext      bytea       NOT NULL,
    wrapped_key     bytea       NOT NULL,
    key_id          text        NOT NULL,
    hint            text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz,
    CONSTRAINT webhook_secrets_owner CHECK (owner_kind IN ('endpoint', 'hook'))
);
CREATE INDEX webhook_secrets_owner ON webhook_secrets (owner_kind, owner_id);
SELECT plux_tenant_policy('webhook_secrets');

-- The delivery log and the durable queue in one: a row per message to a
-- channel or an endpoint. A worker claims due rows with a lease, so a
-- crash retries after the lease and delivery is at least once.
CREATE TABLE notification_deliveries (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    kind            text        NOT NULL,
    channel_id      uuid        REFERENCES notification_channels (id) ON DELETE CASCADE,
    endpoint_id     uuid        REFERENCES webhook_endpoints (id) ON DELETE CASCADE,
    event_id        uuid        NOT NULL,
    event_type      text        NOT NULL,
    recipient       text        NOT NULL DEFAULT '',
    payload         bytea       NOT NULL,
    state           text        NOT NULL DEFAULT 'pending',
    attempts        integer     NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    last_status     integer     NOT NULL DEFAULT 0,
    last_error      text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    delivered_at    timestamptz,
    CONSTRAINT notification_deliveries_kind CHECK (kind IN ('channel', 'webhook')),
    CONSTRAINT notification_deliveries_state CHECK (state IN ('pending', 'delivered', 'failed')),
    CONSTRAINT notification_deliveries_target CHECK ((kind = 'channel') = (channel_id IS NOT NULL) AND (kind = 'webhook') = (endpoint_id IS NOT NULL))
);
CREATE INDEX notification_deliveries_due ON notification_deliveries (next_attempt_at) WHERE state = 'pending';
CREATE INDEX notification_deliveries_log ON notification_deliveries (organization_id, created_at DESC, id DESC);
SELECT plux_tenant_policy('notification_deliveries');

-- How many notifications an organisation created in one hour, for
-- notifications.perHour (LIM-001).
CREATE TABLE notification_hours (
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    hour            timestamptz NOT NULL,
    n               integer     NOT NULL DEFAULT 0,
    dropped         integer     NOT NULL DEFAULT 0,
    PRIMARY KEY (organization_id, hour)
);
SELECT plux_tenant_policy('notification_hours');

-- A cron schedule on a function alias (FN-025). next_fire_at is the next
-- instant it is due, in UTC; the worker advances it.
CREATE TABLE function_triggers (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id  uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    function_name   text        NOT NULL,
    alias           text        NOT NULL,
    expression      text        NOT NULL,
    timezone        text        NOT NULL DEFAULT 'UTC',
    input           bytea       NOT NULL DEFAULT ''::bytea,
    enabled         boolean     NOT NULL DEFAULT true,
    next_fire_at    timestamptz NOT NULL,
    last_fire_at    timestamptz,
    last_result     text        NOT NULL DEFAULT '',
    created_by      uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (environment_id, function_name, alias, expression, timezone)
);
CREATE INDEX function_triggers_due ON function_triggers (next_fire_at) WHERE enabled;
SELECT plux_tenant_policy('function_triggers');

-- One row per fire time of a trigger: the primary key makes a fire
-- idempotent when two workers race or one restarts (FN-025).
CREATE TABLE function_trigger_fires (
    trigger_id      uuid        NOT NULL REFERENCES function_triggers (id) ON DELETE CASCADE,
    fire_at         timestamptz NOT NULL,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    result          text        NOT NULL DEFAULT 'started',
    started_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (trigger_id, fire_at)
);
SELECT plux_tenant_policy('function_trigger_fires');

-- An inbound webhook of a function at /hooks/<app>/<function> (FN-025).
-- A device-facing request names only the app and the function, so the
-- 'hook' scope lets the row be found before the organisation is known;
-- every other table is read after the transaction is rescoped to it.
CREATE TABLE function_hooks (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id  uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    function_name   text        NOT NULL,
    alias           text        NOT NULL,
    verifier        text        NOT NULL DEFAULT 'standard-webhooks',
    enabled         boolean     NOT NULL DEFAULT true,
    created_by      uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (app_id, function_name)
);
ALTER TABLE function_hooks ENABLE ROW LEVEL SECURITY;
ALTER TABLE function_hooks FORCE ROW LEVEL SECURITY;
CREATE POLICY plux_tenant ON function_hooks
    USING (organization_id = plux_current_organization() OR plux_in_scope('hook'))
    WITH CHECK (organization_id = plux_current_organization());

-- The webhook-ids an inbound hook has accepted, for replay protection.
CREATE TABLE function_hook_seen (
    hook_id         uuid        NOT NULL REFERENCES function_hooks (id) ON DELETE CASCADE,
    webhook_id      text        NOT NULL,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    seen_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (hook_id, webhook_id)
);
CREATE INDEX function_hook_seen_age ON function_hook_seen (seen_at);
SELECT plux_tenant_policy('function_hook_seen');
