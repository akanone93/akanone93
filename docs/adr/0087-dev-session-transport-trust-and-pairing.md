<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0087. Dev-session transport, trust and pairing

- **Status:** Accepted (maintainer, 2026-10-22; P10 plan D-1, D-2, D-5, D-7)
- **Date:** 2026-10-22
- **Requirements:** `DEV-002`, `DEV-003`, `DEV-010`, `DEV-021`, `SEC-021`, `SEC-052`, `SEC-056`, `SEC-103`, `GOV-032`, `LIM-001`

## Context and problem

A developer saves a page and sees it on a real phone half a second later (`DEV-010`). That needs a
push channel from the server to the device, a way for the device to trust what it is pushed
without weakening verify-before-load (`SEC-052`), and a way to bind a device to the developer's
session without a long-lived credential. Spec §20 draws the channel as a WebSocket. The server
has no WebSocket library and its allowlist has none; the runtime already has a `dart:io`
WebSocket transport (`DAT-001`). A draft is not a release: it has no TUF metadata chain and no
anti-rollback sequence (`SEC-055`). The question: how do the channel, the trust in dev bundles and
the pairing work, and what does creating a session require?

## Decision drivers

- Security and correctness first: a device that can be driven remotely is a remote-control surface.
- `SEC-052` holds unchanged: nothing beyond a container header is parsed before signature and hashes are verified.
- `SEC-056`: production runtimes reject development-signed material, and the reverse.
- The runtime's HTTP clients (`cronet_http`, `cupertino_http` behind `package:http`) and the browser's `fetch` (Studio, P11) cannot stream in both directions on one request; load balancers often reach the server over HTTP/1.1.
- Layering: only `signing/` touches private keys (L-3); `worker` compiles without signing capability (L-4); handlers stay thin (L-1).
- Limits come from the registry (`LIM-001`); nothing is hard-coded.

## Considered options

1. WebSocket for the device channel, unary ConnectRPC for every command, a DPoP-bound socket ticket, and dev bundles that are mini releases signed with a development key and verified against pinned development roots.
2. ConnectRPC bidirectional streaming for the whole channel.
3. Server-Sent Events down and unary POST up.
4. Unsigned dev sections over the authenticated socket.

## Decision

Chosen option: **1**, because it is what spec §20 draws, reuses the transport the runtime already
has, keeps `SEC-052` intact and adds one reviewed server dependency.

- **Transport.** The server serves the device channel with `github.com/coder/websocket` (ISC; a
  `dependencies.md` row lands with the contracts milestone). The device uses `dart:io`'s
  `WebSocket` through the transport in `packages/plux_flutter/lib/src/data/stream_transport.dart`.
  Events are `DevEvent` protobuf messages, one per frame, each with a session sequence number
  (ADR-0090); the device sends their canonical proto3 JSON mapping in text frames, as its
  ConnectRPC calls already do, and the server decodes them into the messages. Every command and query is a unary ConnectRPC call of `DevSessionService`
  (`CreateSession`, `GetPairing`, `Pair`, `ListDevices`, `EndSession`, `PushPatch`, `SendCommand`,
  `ExportTrace`); the CLI reads the stream through the server-streaming `WatchEvents`. The api
  role's ingress allows the upgrade and an idle timeout from configuration (Appendix H).
- **Dev bundles.** A dev bundle is a mini release, a manifest plus sections, signed by `signing/`
  with the development targets key. The Dev app and debug host builds pin development root keys
  supplied at build time. The device applies dev bundles to a **session overlay** over the active
  release: the overlay never writes the production store, has its own sequence space and is
  discarded when the session ends. The manifest gains an additive `development_session` field
  (session ID and sequence space) that release manifests never carry and release runtimes
  refuse. Release builds do not contain the code path (ADR-0091).
- **Compile and sign.** `PushPatch` validates the document patch; the worker compiles it
  incrementally (ADR-0089) without any signing capability (L-4); the sections are then signed by
  a separate call into `signing/` with the development class (L-3) and pushed to every paired
  device of the session.
- **Pairing token.** A compact signed token (the development token key, through `signing/`)
  carrying session ID, user, app, environment, expiry and a single-use nonce. The QR holds
  `plux-dev://pair?server=<url>&token=<token>`. Its lifetime is `devSession.pairingTokenSeconds`
  (at most 120 s, `DEV-002`). The server stores the token's hash and consumes it in one
  transaction; a second use is `PLX-9202`, an expired token `PLX-9201`. Up to
  `devSession.devicesPerSession` devices pair to one session.
- **Device authentication on the socket.** The device pairs over unary ConnectRPC under DPoP
  (`SEC-021`) and, where the platform gives it, development attestation. The reply carries a
  short-lived socket ticket bound to the session and to the thumbprint of the device's DPoP key.
  The device presents it once at the WebSocket upgrade (`PLX-9205` when invalid). The socket closes
  when the session ends, the ticket expires or the device is revoked. Per-message DPoP is not used:
  the socket authenticates once.
- **Creating a session.** Only for environments whose kind is not production and which the
  installation enables for dev sessions (`DEV-003`, `PLX-9203`). Permissions `devsession.create`
  (Developer and above: start, push, command) and `devsession.observe` (read the stream).
  Step-up (`SEC-103`) applies only when the organisation's policy requires it for the
  environment (ADR-0075, Revision). Access control is a security capability, so all of it is core
  (`GOV-032`).
- **Relay.** Per-device bounded queues from the limits registry, with backpressure and fan-out;
  commands are authorised per device (`PLX-9206`) and rate-limited (`devSession.commandsPerMinute`);
  every state change is audited. The relay merges the server logs of a device's function calls by
  trace ID for the session's own app and environment only (D-7).
- **Errors.** `PLX-9200`–`9209`, listed in Appendix F.

## Consequences

- **Positive:** the device keeps one transport it already has; commands stay unary and
  inspectable; verify-before-load holds for dev bundles; a stolen pairing token is single-use and
  short-lived; a stolen socket ticket is useless without the device's key.
- **Negative:** one new server dependency; a second signing class and a second set of pinned
  roots in debug builds; long-lived sockets need ingress idle-timeout settings and a chart test.
- **Follow-up:** migrations for sessions, pairing tokens and devices (D2); `devsession.proto` and
  the authorisation matrix entries; the `dependencies.md` row; a WebSocket round trip through the
  ingress in the `e2e-kind` chart test.

## Options in detail

### Option 1 — WebSocket plus unary ConnectRPC

Described above. A hand-written RFC 6455 server was considered inside this option and rejected:
framing code in the security path is not worth saving a reviewed dependency.

### Option 2 — ConnectRPC bidirectional streaming

Keeps one protocol, but the runtime's HTTP clients and the browser cannot stream both ways on one
request, and HTTP/1.1 intermediaries break it. The runtime would need a second client stack.
Rejected.

### Option 3 — SSE down, POST up

Smallest and DPoP-per-request, but weak for the high-rate device-to-server event stream, and the
device would hold two connections. Rejected.

### Option 4 — unsigned dev sections

Contradicts `SEC-052`: a socket is not a signature. Rejected.

## Revision (2026-10-23, P10: compression on the device socket)

The device socket negotiates permessage-deflate with context takeover. A dev bundle is sent whole
and differs little from the previous one, and uncompressed it did not reach the device within
the 500 ms of `DEV-010` over the good Wi-Fi profile (619 ms p95 for server and transport, against
231 ms compressed; `docs/benchmarks/devloop.md`). The runtime's `dart:io` WebSocket offers the
extension by default. The frames carry no secret: the pairing token and the socket ticket travel
in the upgrade's headers, so compression cannot leak one through frame sizes.
