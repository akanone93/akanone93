-- SPDX-FileCopyrightText: 2026 Plux contributors
-- SPDX-License-Identifier: AGPL-3.0-only

-- Sessions an administrator can see and end, and the step-up a session
-- carries (GOV-005, SEC-103, ADR-0075). Every change is an added column
-- that is nullable or has a default, so a replica of the previous version
-- keeps working during a rolling upgrade (SRV-021).

-- last_seen_at is refreshed at most once a minute and measures the idle
-- timeout; created_at measures the absolute one, which expires_at already
-- holds. ip_prefix is the /24 (IPv4) or /48 (IPv6) the last request came
-- from, never the full address. step_up_at is when a WebAuthn assertion
-- with user verification last succeeded in the session. idle_timeout_ms is
-- the idle timeout in force for the session, refreshed with last_seen_at
-- from the limits of the organisations its user belongs to; NULL means the
-- installation's default.
ALTER TABLE sessions
    ADD COLUMN last_seen_at    timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN step_up_at      timestamptz,
    ADD COLUMN ip_prefix       text        NOT NULL DEFAULT '',
    ADD COLUMN user_agent      text        NOT NULL DEFAULT '',
    ADD COLUMN idle_timeout_ms bigint;

-- Tokens record where they were last used, so that the same list shows
-- sessions and tokens.
ALTER TABLE access_tokens
    ADD COLUMN last_ip_prefix  text NOT NULL DEFAULT '',
    ADD COLUMN last_user_agent text NOT NULL DEFAULT '';
