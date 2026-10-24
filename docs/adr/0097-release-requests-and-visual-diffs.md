<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0097. Release requests and visual diffs

- **Status:** **Accepted** (maintainer, 2026-10-24; P11 plan M-9)
- **Date:** 2026-10-24
- **Requirements:** `STU-100`, `STU-102`, `GOV-020`

## Context and problem

Studio's publish wizard has nine steps from selecting plugins to releasing (F11), including builds,
reports, visual diffs, approval and a rollout plan. If the wizard held that state, closing the tab
would lose it and two people could publish the same app at once. Separately, `STU-100` requires a
visual diff "rendered server-side on standard devices", which needs the Flutter renderer on the
server, a large toolchain that must not sit beside credentials. See
[the functional decisions](../plans/p11-studio-functional-decisions.md) F11 and F12 and
[the P11 plan](../plans/p11.md) §4.3 (contracts #8 and #9), O-9.

## Decision drivers

- Every step's result is stored on the server; the wizard only displays it.
- An approval means "this exact content", not "this app" (`GOV-022`).
- Retries and double clicks are safe.
- A rendering toolchain must not widen the trust surface (layering L-4).
- Rendering must not be a way to exhaust the server.

## Considered options

1. A server-side release request state machine and a headless render worker in its own image.
2. Wizard state in the browser, with a single publish call at the end.
3. Visual diffs rendered in the browser from canvas snapshots.

## Decision

Chosen option: **1**.

### `ReleaseRequestService` (contract #8)

- A **release request** is a server resource with the states of F11: select, freeze and build,
  reports, diffs, changelog, target, approval, rollout plan, release. End states are Released,
  Rejected, Expired and Cancelled; Failed and Blocked return to the step that failed.
- **Idempotent**: every transition and the build job carry an idempotency key.
- **One open request per app and channel.** A second attempt returns the open one so that Studio
  can show "Anna is publishing" and let others view it.
- **Approval is bound to the content hash** of the release. If the content changes, earlier
  approvals become invalid and the request returns to the build step.
- It is **linked to `ApprovalService`** ([ADR-0076](0076-approval-engine.md)), which evaluates the
  policies; the request does not duplicate them. Production release still needs the WebAuthn
  step-up (`SEC-103`).
- Jobs keep running while the tab is closed. Progress is delivered on the events stream
  ([ADR-0096](0096-studio-live-channels.md)). Cancelling calls `CancelPublishJob`; versions already
  built stay, being immutable.
- The approval policy simulator ("who would need to approve this change?", contract #19) answers
  from the same policy evaluation.

### Visual diffs (contract #9; O-9)

- A visual diff is a **render job**: it runs the P8 golden harness
  ([ADR-0070](0070-golden-tests.md)) headless with `flutter_tester`, for each changed screen on
  the standard devices, and stores the images and the result.
- The job runs in a **separate, signed render-worker image** holding the Flutter SDK, fonts and the
  runtime. It has **no database credentials and no signing capability**
  (L-4), like `fnrunner` and `builder`.
- Jobs are **queued and rate-limited per organisation** by `render.visualDiffJobsPerOrganization`.
  Approvers need a finished diff before the request can ask for approval (`GOV-023`).

## Consequences

- **Positive:** publish survives closing the tab; two people cannot publish one app and channel at
  once; the renderer sits behind the same isolation rules as other workers.
- **Negative:** a second image to build, sign, scan and ship; render time adds to publishing; fonts
  and the Flutter version must be pinned so that diffs are stable.
- **Follow-up:** the render-worker image in `deploy/` and the release pipeline; the limit key;
  error codes for the request states.

## Options in detail

### Option 2 — wizard state in the browser

Loses work with the tab, cannot enforce one open request, and cannot bind an approval to content.
Rejected.

### Option 3 — render in the browser

Rendering with the canvas's CanvasKit differs from the device runtime, and the reviewer would
approve an image the author produced. The specification asks for server-side rendering. Browser
renders remain for thumbnails only (Studio decisions A5.6). Rejected for diffs.
