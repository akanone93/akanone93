<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0101. Structural session replay

- **Status:** Proposed (maintainer decision 2026-10-28, plan P11 §4.3 O-8; S8.6)
- **Date:** 2026-10-28
- **Requirements:** `ANL-050`, `SEC-161`, `SCH-012`, `SEC-092`, `SEC-162`, `ANL-003`, `LIM-001`

## Context and problem

`ANL-050` asks for opt-in session replay that records page, node and state changes and not video,
so that Studio can replay them on the canvas. `SEC-161` names a `replay` consent category that no
code yet honours, and `SCH-012` requires sensitive values to be excluded from session replay. Pages
are declarative, so what a user saw is already described by the release: node identifiers, widget
types and state keys. A recording therefore needs only what the release does not hold: which
screen was open when, where each node was laid out, which node the user touched, and which state
entries changed.

## Decision drivers

- Nothing sensitive may be recorded, however a page is built (`SCH-012`, `SEC-092`).
- The device decides first; the server never trusts it (`ANL-003`, `SEC-161`).
- Recording costs nothing when it is off, and never does I/O, encoding or hashing on the UI isolate.
- Every size and count comes from the limits registry (`LIM-001`).
- A replay belongs to one device and is erased with it (`SEC-162`).

## Considered options

1. Pixel or video capture (screenshots on a timer).
2. Structural recording: node identifiers, widget types, rectangles, event kinds and non-sensitive
   scalar state values; Studio redraws the page from the release.
3. Reuse the telemetry event stream with extra event names.

## Decision

Chosen option: **2**, as a separate `ReplayService`, because pixels cannot be redacted reliably
(option 1) and replays are larger, rarer and kept shorter than telemetry events, so they need their
own limits, storage and retention (option 3).

**What is recorded.** A replay is an ordered list of events, each with a millisecond offset from
its start:

| Kind | Fields |
|---|---|
| `screen` | route, plugin key |
| `layout` | nodes of the shown page: path, widget type, rectangle in logical pixels, `masked` |
| `tap` | node path, position inside the node as a fraction |
| `input` | node path only; the text is never read |
| `state` | state entry name, value type, and the value only for `bool`, `int` and `double` |

No string value of any state entry, no text of any node, no request, header, body or parameter, and
no image is recorded. A node whose widget is a text input, a secret input or a secure-UI widget, and
any node inside one, is `masked`: its rectangle is kept, nothing else. A state entry tagged
`sensitive` is not recorded at all, not even its name.

**Consent and settings.** Recording starts only while the host's consent has `replay` set
(`PluxConsent.replay`, off by default) and the security setting `allowSessionReplay` is true. The
setting is true under `standard` and `strict` and false under `maximum`, so replay is disabled and
cannot be enabled there (`ANL-050`). Withdrawing `replay` stops recording at once and deletes every
unsent replay on the device. The upload carries the consent (`Consent.replay`) and the server
refuses a replay when it is missing or false.

**Sampling and bounds.** The host sets a sampling rate for `replay` like any telemetry event
(`PluxConfig.telemetrySampling`, default 0.1 here); one replay covers one foreground stretch. Limits
in `schema/limits.json`, all reported with their default and maximum:

| Limit | Default | Max | Enforced by |
|---|---|---|---|
| `replay.perDevicePerDay` | 5 | 100 | runtime, server |
| `replay.eventsPerReplay` | 2000 | 20000 | runtime, server |
| `replay.bytesPerReplay` | 262144 | 4194304 | runtime, server |
| `replay.retentionDays` | 7 | 90 | server |

The runtime stops adding events at either bound and closes the replay, marked `truncated`; the
server refuses a replay above either bound, and the surplus above the daily count.

**Off the UI isolate.** The UI isolate keeps a bounded list of small maps (as telemetry events do)
and, when the replay closes, posts it once to the sync isolate, which encodes it, stores it in a
bounded outbox and uploads it gzip-compressed with the device's token and DPoP proof, the same
path as `TelemetryService.IngestEvents`.

**Server.** `ReplayService.IngestReplay` is a device RPC. The server re-applies every rule: it
drops any state event whose name `telemetry.Sensitive` flags, any string value, any field not in
the table above, any text longer than the telemetry text bound, any child of a masked node, and
rejects a replay without consent, above a limit, or for another app. Replays are stored per
organisation and app in `session_replays` under the tenant and app row-level policies, with an
`expires_at` set from `replay.retentionDays` and deleted by a scheduler job. `ListReplays` and
`GetReplay` need `analytics.read` on the app. The table is device-keyed in the personal data map and
is exported and erased with the device (`SEC-162`).

**Studio.** Screen `A47`, `/apps/$app/analytics/replays/:replayId`, plays the events back on the
canvas with a timeline; the list is a tab of the analytics area. The page is drawn from the
release's page document through the same scene path as the plugin canvas, with the recorded
rectangles for masked nodes.

## Consequences

- **Positive:** a replay holds no value a user typed or a page showed; its size is bounded on the
  device and again on the server; turning consent off removes the data on the device.
- **Negative:** a replay shows layout and interaction, not the exact look of a custom-painted
  native widget; state strings are not shown.
- **Follow-up:** scroll offsets and animation frames are not recorded; they can be added as new
  event kinds without a format change.

## Options in detail

### Option 1: pixels

Exact, but screenshots carry whatever is on screen, including values a page never tagged
sensitive, and cost encoding on a device. Rejected on `SCH-012`.

### Option 3: telemetry events

No new service, but a replay does not fit the scalar-field event model, its limits differ, and
mixing it into the rollups would count it as analytics. Rejected.
