-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- The releases promoted to an environment's channels (REL-004, build log row
-- 322). A release is made in one environment (releases.environment_id) and
-- promoted to the channels of others, but a channel keeps only the release it
-- points at now, so an environment's timeline could not list a release
-- promoted to it — the usual way into production — once the channel moved
-- on. Expand-only: rows and code of the previous version stay valid. Nothing
-- is backfilled: the listing also counts the release a channel points at now
-- and the candidates of the environment's rollouts.

-- One row per release a channel was pointed at, kept from the first time.
CREATE TABLE channel_promotions (
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    app_id          uuid        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    environment_id  uuid        NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    channel_id      uuid        NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    release_id      uuid        NOT NULL REFERENCES releases (id) ON DELETE CASCADE,
    first_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (channel_id, release_id)
);
CREATE INDEX channel_promotions_environment ON channel_promotions (environment_id, release_id);
SELECT plux_tenant_policy('channel_promotions');
SELECT plux_app_policy('channel_promotions');
