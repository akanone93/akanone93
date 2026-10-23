<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0090. Dev stream event schema, redaction and the trace file

- **Status:** Accepted (maintainer, 2026-10-22; P10 plan §2.2, §5.4)
- **Date:** 2026-10-22
- **Requirements:** `DEV-020`, `DEV-021`, `DEV-041`, `FN-036`, `ACT-030`, `ACT-031`, `SCH-012`, `SEC-092`, `LIM-001`

## Context and problem

A running page produces logs, action runs, state changes, network and function calls, sync events,
frame timings and errors. A developer must see them live, filter and search them, pause the view,
and hand a colleague a file that reproduces what they saw (`DEV-020`, `DEV-021`). The stream
leaves the device, so it must never carry what the rest of the system forbids to log: keys,
tokens and fields tagged `sensitive` (`SCH-012`, `SEC-092`), even in development. A phone on a
flaky network must not run out of memory or slow the frame it is measuring. The question: what
is an event, where is it redacted, how is it bounded and resumed, and what is in a trace file?

## Decision drivers

- Sensitive values never leave the device.
- Bounded memory and no UI-isolate cost; the stream must not disturb the timings it reports.
- A schema both Go and Dart generate from one source, evolved additively.
- The same data for the CLI today and Studio's bottom drawer in P11.

## Considered options

1. Protobuf messages, one per WebSocket frame (the device sends the canonical proto3 JSON mapping in text frames, as its ConnectRPC calls do, so the runtime needs no protobuf library; the CLI stream and the trace file carry binary protobuf), redacted at the source, bounded ring buffers with sequence numbers; a trace file as a gzip stream of length-prefixed messages.
2. JSON text frames and a JSON-lines trace file.
3. Redaction at the relay instead of the device.
4. A debugging protocol borrowed from an existing tool (for example the Chrome DevTools Protocol).

## Decision

Chosen option: **1**.

- **Schema.** `proto/plux/v1/devevents.proto` holds messages only. A `DevEvent` carries a session
  sequence number and one of: log, action trace (the process flow,
  `ACT-030`), state diff (bounded by `devtools.stateDiffBytes`), network call (method, URL
  template, status, timings), function call (`FN-036`), sync event, frame timing, error with
  node path (from the dev bundle's source map), and `plux_security` detection (ADR-0091). One
  event per frame; an event above `devtools.eventBytes` is dropped with `PLX-9209`.
- **Redaction at the source (`ACT-031`, `SCH-012`).** The device redacts before encoding.
  Request and response bodies and headers are off by default and, when switched on, are redacted
  like every other value. The relay and the CLI never see a sensitive value, so no later stage
  has to be trusted to remove it.
- **Bounds and resume (`DEV-021`).** Per-device ring buffers bounded by
  `devtools.streamBufferEvents` and `devtools.streamBufferBytes`, with a drop counter reported in
  the stream. Pausing a view keeps buffering up to the bound. After a reconnect the client asks
  for events after a sequence number and receives what the buffers still hold, plus the drop
  count for the rest.
- **Function logs (D-7).** The relay merges server function logs into a device's function-call
  event by trace ID, for the session's own app and environment only.
- **Trace file.** A gzip-compressed stream of length-prefixed `DevEvent` messages, preceded by a
  header message: format version, session, app, environment and export time. `plux dev trace
  export` writes it, `plux dev trace show` reads it, and Studio reads it later. An exported trace
  is redacted exactly like the stream; the server writes nothing to it that the stream did not
  carry. A file that cannot be read is `PLX-9233`.
- **Encoding off the UI isolate.** Capture, redaction, encoding and the socket run in a background
  isolate (L-6).

## Consequences

- **Positive:** a compact, versioned, additive schema; redaction cannot be bypassed by a
  forgetful relay; the trace file is the same data as the live stream, so a bug report replays
  what the developer saw.
- **Negative:** the device's JSON frames are larger than binary protobuf would be; a protobuf change
  needs `make gen`; redaction at the source costs device time, which is why it runs off the UI
  isolate.
- **Follow-up:** the limit keys in the registry; a redaction test for every event type; P12 uses
  the trace file as input for debugging help.

## Options in detail

### Option 1 — protobuf and length-prefixed trace

Described above. Matches the API's contract language (ADR-0005) and shares generated types.

### Option 2 — JSON frames

Readable, but larger, slower to encode on a phone and without a schema that both sides generate.
Rejected.

### Option 3 — redact at the relay

One implementation, but sensitive values would cross the network and sit in relay memory.
Rejected: `SEC-092` forbids it.

### Option 4 — an existing protocol

Would bring a large surface that does not match Plux's nodes, actions or sections. Rejected.
