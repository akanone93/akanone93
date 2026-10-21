<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Notifications and Webhooks Guide

How Plux tells people, chat tools and your own systems about what happened: the in-app inbox,
email, chat channels, and signed outbound webhooks. The requirements are `SRV-062`, `SRV-063`,
`FN-025`, `SEC-105` and [ADR-0081](../adr/0081-notifications-webhooks-and-function-triggers.md).

## 1. The idea in one page

A change in Plux publishes a typed **event**: an approval was requested, a release was published,
a rollout was paused, a break-glass action was used, a function is failing, an alert fired, a
licence is expiring. The notification service turns each event into deliveries, one for each
subscription that wants it:

- an **inbox** entry for each subscribed person;
- a **channel** delivery for each subscribed channel (email, Slack, Microsoft Teams, Telegram);
- a **signed webhook** delivery for each outbound webhook endpoint.

A worker sends the deliveries. A failed delivery is retried, and every delivery is recorded in a
log you can read.

## 2. Event types

`NotificationService.ListEventTypes` lists the event types. The ones in the core are:

- approval requested;
- release published;
- rollout paused;
- lock requested;
- function failing;
- alert fired;
- break-glass used;
- licence expiring.

## 3. Subscriptions and the inbox

A person subscribes to event types with `NotificationService.Subscribe` and leaves with
`Unsubscribe`; `ListSubscriptions` shows what they receive. Subscriptions can be per person or per
team, so the on-call team hears about a paused rollout without everyone in the organisation doing
so. The inbox is the list of a person's notifications, read with `ListInbox` and marked with
`MarkRead`.

## 4. Channels

Channels are where a delivery goes outside the inbox. Create one with
`NotificationService.CreateNotificationChannel`, test it with `TestNotificationChannel`, and remove
it with `DeleteNotificationChannel`. `ListNotificationChannels` shows them.

| Channel | Notes |
|---|---|
| Email (SMTP) | TLS is required, STARTTLS or implicit TLS. An insecure development setting is possible only when the installation allows it, and it logs a distinct warning on every use. |
| Slack | A Slack webhook. |
| Microsoft Teams | A Teams webhook. |
| Telegram | A Telegram webhook. |

A channel that is misconfigured is refused with `PLX-8400`. Channel credentials and webhook secrets
are stored under envelope encryption and never appear in a log, an error, an API response or the
delivery log (`SEC-092`).

Break-glass alerts use these channels, so approvers hear about an emergency at once
([governance guide](governance.md), section 9).

## 5. Outbound webhooks

A webhook is an HTTPS endpoint of yours that receives each event as a signed message. Create one
with `WebhookService.CreateWebhook`; `ListWebhooks`, `DeleteWebhook` and `ListWebhookDeliveries`
manage and inspect them.

### Signature

Plux signs every message with the **Standard Webhooks** scheme, so any Standard Webhooks library
verifies it. Each message carries three headers:

- `webhook-id`: a unique identifier for the message, the same on every retry;
- `webhook-timestamp`: when it was signed;
- `webhook-signature`: an HMAC-SHA256 over the identifier, the timestamp and the body.

Verify the signature with the secret of the endpoint, and reject a timestamp that is too old, so a
captured message cannot be replayed. The secret can be rotated with
`WebhookService.RotateWebhookSecret`. During a rotation the old and the new secret both verify, so
you can switch your receiver without losing messages.

### Retries

A delivery that fails is retried with exponential backoff and jitter for **24 hours**. After that it
is given up and recorded as `PLX-8401`, visible in `ListWebhookDeliveries`. Delivery is at least
once: a receiver must treat a repeated `webhook-id` as a duplicate.

### Destinations

- An endpoint in a private or loopback address range is refused when it is registered, and again
  when the connection is made (`SEC-105`).
- Every destination is on the [egress list](../operations/egress.md).
- The number of webhooks per organisation is bounded by `webhooks.perOrg` (default 50).

## 6. Limits

Publishing is bounded per organisation by `notifications.perHour` (default 600). Notifications over
the limit are dropped, counted, and audited once an hour, so a flood cannot drown the channels
silently. The limits are in the [limits reference](../reference/limits.md).

## 7. Function triggers

Serverless functions ([functions guide](functions.md)) can run on a cron schedule and be triggered
by inbound webhooks at `/hooks/<app>/<function>`. An inbound webhook is checked with the same
Standard Webhooks signature, and a replayed message is refused (`PLX-7151` for a bad signature,
`PLX-7152` for a replay). A trigger carries no credentials; the function runs under the isolation
rules of the function runner.

## 8. Errors

| Code | Meaning |
|---|---|
| `PLX-8400` | A notification channel is misconfigured. |
| `PLX-8401` | A webhook delivery was given up after 24 hours of retries. |
| `PLX-7151` | An inbound function webhook has a bad signature. |
| `PLX-7152` | An inbound function webhook is a replay. |
