<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0096. Studio live channels and the shared stream layer

- **Status:** **Accepted** (maintainer, 2026-10-24; P11 plan M-9)
- **Date:** 2026-10-24
- **Requirements:** `STU-002`, `STU-075`, `SRV-062`, `DEV-021`

## Context and problem

Studio must show lock changes, new revisions, notifications, permission changes and publish
progress as they happen, and the Dev app's sessions need events and frames in both directions.
Today `WatchPublish` is the only server stream and it polls; locks are unary leases; the only
WebSocket code is the device data-source gateway; the server speaks HTTP/1.1 and h2c. Two
constraints shape the answer: browsers cannot call client-streaming RPCs, and the server runs as
several `api` replicas, so an event produced on one replica must reach a stream held by another.
Background: [Studio decisions](../plans/p11-studio-decisions.md) A5.2 and
[the P11 plan](../plans/p11.md) §4.3 (contracts #1 and #2) and S0.5.

## Decision drivers

- One protocol, typed by protobuf, behind the same authentication and CSRF path as every call.
- Resumable after a dropped connection, with bounded server buffering (`DEV-021`).
- Works on a multi-replica install and on the in-memory, single-node install without a code fork.
- No new infrastructure, and no state held only in one process (M-8).
- A session must not stay alive because a background stream is open (O-1).

## Considered options

1. Connect server streaming through the BFF on a shared, resumable stream layer in `internal/cache`.
2. WebSocket between browser and server.
3. Polling with unary calls.
4. PostgreSQL `LISTEN`/`NOTIFY` as the fan-out and the buffer.

## Decision

Chosen option: **1**, because it reuses the Connect stack and the cache and notifier already in the
server, and gives resumption and bounds in one place.

- **Server streaming, no WebSocket in the browser.** Server to browser is typed Connect server
  streams; browser to server is unary RPCs. A WebSocket would add an untyped second protocol with
  its own upgrade, origin checks and authentication. HTTP/2 runs between browser and BFF so that
  streams do not exhaust the six-connection limit of HTTP/1.1. The BFF proxies streams unbuffered,
  passes the session through, sends heartbeats and sets deadlines.
- **One events stream per tab.** `StudioEventsService.Watch` carries lock and revision changes,
  notifications and the unread count, permission changes and publish progress (contract #1). It
  resumes from a cursor (the entry ID of the last event seen) and is bounded by
  `studio.eventsStreamBufferEvents`; a cursor older than the buffer returns an explicit "resync"
  event, and the client refetches.
- **One device stream per paired device.** `DevSessionService.WatchEvents` takes `after_sequence`
  and resumes from it ([ADR-0087](0087-dev-session-transport-trust-and-pairing.md)). Browser to
  device commands are unary RPCs within the 500 ms of `DEV-010`.
- **The shared stream layer.** `internal/cache` gains **resumable reads by entry ID** (`XREAD` and
  `XRANGE`, beside the existing consumer-group reads) on both backends: Valkey, and the in-memory
  backend of single-node installs. Streams are bounded by `MAXLEN` and byte limits. Readers are
  woken by the existing cross-replica `cache.Notifier`
  ([ADR-0086](0086-restriction-policies-and-effective-permissions.md), which uses pub/sub), with a
  periodic read as the fallback for a lost notice. A notice is only a wake-up; the stream is the
  truth, so a lost or duplicated notice changes latency, never content.
- **Dev-session state uses the same layer** to cross replicas (S0.9; Revision of ADR-0087).
- **Uploads.** Browsers cannot client-stream, so the client-streaming `UploadAsset`,
  `ImportDraft` and `UploadSource` get unary chunked variants beside them (contract #2).
- **Sessions idle on user activity only (O-1).** Open streams, heartbeats and lock renewals do not
  extend a session. A session that expires closes its streams with an authentication error, which
  Studio turns into the sign-in popup that keeps unsent changes.

## Consequences

- **Positive:** one mechanism for Studio events, device events and dev-session frames; testable
  with two replicas and without Valkey; the BFF stays a thin proxy.
- **Negative:** `internal/cache` grows an interface that both backends must implement and a test
  suite that both must pass; stream bounds mean a long-offline client resyncs rather than replays.
- **Follow-up:** the limit key and error codes in the registries; the periodic-read interval as a
  setting; `WatchPublish` moves onto the layer when S7 touches it.

## Options in detail

### Option 2 — WebSocket

Bidirectional, but untyped, with a second authentication and CSRF path, and the commands that
need it already fit in unary calls. Rejected.

### Option 3 — polling

Simple, but lock changes and notifications would lag by the interval and load the server in
proportion to open tabs. Rejected.

### Option 4 — PostgreSQL `LISTEN`/`NOTIFY` as the buffer

Notifications are not stored, so a reconnecting stream could not resume, and the payload is
limited to a few kilobytes. The plan first proposed it as the fan-out; the existing notifier
already does that job without tying streams to one database connection. Rejected as the buffer.
