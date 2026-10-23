# 0081. Notifications, Standard Webhooks and function triggers

- **Status:** Accepted
- **Date:** 2026-10-18
- **Requirements:** `SRV-062`, `SRV-063`, `FN-025`, `SEC-105`, `SEC-163`, `GOV-024`

## Context and problem

Approvals, break-glass, rollouts, alerts and licence expiry all need to reach people where they
work, and external systems need signed events. Functions need to run on a schedule and on inbound
webhooks (`FN-025`, optional but accepted into P9 by the maintainer). All outbound calls are a
server-side request forgery risk and must appear in the egress list (ADR-0082). The questions:
which channels, which webhook standard, what delivery guarantees, and how are triggers
authenticated?

## Decision drivers

- Reach people through the tools regulated teams already use, with no SDK dependency.
- A webhook signature scheme that receivers can verify with existing libraries.
- At-least-once delivery with visibility into failures.
- SSRF-safe clients (`SEC-105`); every destination configured and listed.
- In-house code where small (cron, SMTP via `net/smtp`, signing).

## Considered options

1. Built-in channels, Standard Webhooks signing, a worker-run scheduler and a verifier interface for inbound hooks.
2. Webhooks only; users bridge to chat and email themselves.
3. A notification or workflow service as a dependency.

## Decision

Chosen option: **1**.

- **Channels (`SRV-062`, core).** In-app inbox (API and CLI; Studio in P11), SMTP, Slack,
  Microsoft Teams and Telegram webhooks. Per-user and per-team subscriptions to event types:
  approval requested, release published, rollout paused, lock requested, function failing,
  alert fired, break-glass used, licence expiring. A misconfigured channel gives `PLX-8400`.
- **Outbound webhooks (`SRV-063`).** Standard Webhooks: `webhook-id`, `webhook-timestamp`,
  `webhook-signature` (HMAC-SHA256), with rotatable secrets that overlap during rotation.
  Exponential backoff with jitter for 24 h, then `PLX-8401`; a delivery log; every destination in
  the egress list. Limits from the registry (`webhooks.perOrg`, `notifications.perHour`).
- **Function triggers (`FN-025`).** Cron schedules (in-house parser, UTC or a zone, `PLX-7150` for
  an invalid expression) run by the worker's scheduler, leader-elected as in ADR-0078. Inbound
  webhooks at `/hooks/<app>/<function>` are verified by Standard Webhooks signatures or a
  provider-specific verifier interface (the later payment provider webhooks, `PAY-006`, plug in
  here) and are replay-protected (`PLX-7151` bad signature, `PLX-7152` replay). Functions run
  under the isolation rules of the function runner (ADR-0064); triggers carry no credentials.
- Break-glass alerts (ADR-0076) use these channels.

## Consequences

- **Positive:** approvals and incidents reach people at once; receivers verify with a public
  standard; triggers add no new execution path, only new callers of the existing runner.
- **Negative:** each channel is code to maintain against third-party formats; delivery retries
  need a durable queue and a log table; inbound endpoints are public attack surface needing rate
  limits and replay protection.
- **Follow-up:** migration 0042; runbook for notification backlogs; documented verifier
  interface for payment providers (P13).

## Options in detail

### Option 1 — built-in channels and Standard Webhooks

Covers the common case with no dependency and a published signing scheme. Cron and signing are
small enough to write and test in-house.

### Option 2 — webhooks only

Least code, but every customer rebuilds the same bridge, and an approval request that sits in an
unread inbox is a governance gap. Rejected.

### Option 3 — a notification service

Another component to run, another egress destination, and not available air-gapped. Rejected.
