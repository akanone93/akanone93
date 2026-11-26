<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0113. The visual diff of the publish flow is parked

- **Status:** Accepted (maintainer decision, 2026-11-26)
- **Date:** 2026-11-26
- **Requirements:** STU-100, GOV-023, TST-003

## Context and problem

The publish flow's fourth step renders a visual diff: before-and-after images of every changed screen, rendered by the render worker on the real runtime (`STU-100`). The diffs step and the approval waited for a finished render (`GOV-023`, `PLX-8062`). A render is all or nothing: one case that does not render fails it.

Publishing Apply Register (§J1, build log rows 299 and 300) showed the render failing for reasons outside the release being reviewed. Row 299 fixed the test harness (device functions, dialog and sheet pages, two runtime defects): 568 of 588 cases render. The remaining 20 cases are five pages the runtime cannot render: three runtime defects (`TextField` with `expands`, `ButtonSegment` icons, `KycCameraCaptureView` in a small box) and two document errors that Studio and the compiler accept. Until each is fixed, no request of the app passes step 4, and so none can be released.

## Decision drivers

- A release must not be blocked by a renderer that cannot render pages the platform accepted.
- Approvers still need evidence of what changes; the JSON diff of the changed documents works and is cheap.
- The parking must be explicit, in one place, and reversible without a migration.

## Considered options

1. Fix the three runtime defects, add compiler errors for the two document cases, fix the documents, then require the render again (row 300, option A).
2. Fix only the five documents.
3. Park the visual diff: the diffs step and approval stop waiting for a render; Studio says the visual diff is coming soon.

## Decision

Chosen option: **3, park the visual diff** (maintainer, 2026-11-26: "I want the diff to be skipped … show a coming soon message in the UI and park it specifically").

- The server passes the diffs step without a render, and `RequestApproval` does not ask for one: `diffsParked` in `backend/internal/releasereq/diff.go`.
- Studio's diffs step says "Visual diff: coming soon", offers no render, and always offers **Continue**; it still shows the JSON diff of the changed documents, and a render a request already has: `visualDiffParked` in `studio/apps/web/src/features/publish/panel.ts`.
- The render worker, the render jobs and `RequestVisualDiff` stay as they are, unused by Studio, so that un-parking is a change of the two constants and their tests.
- The request's state machine is unchanged (the `diffs` step remains), so stored requests need no migration.

## Consequences

- **Positive:** releases are no longer blocked by the renderer; the review keeps the JSON diff.
- **Negative:** reviewers and approvers do not see before-and-after images. `GOV-023` (approvers see the visual diff) is `WIP` again; `STU-100` stays `WIP`.
- **Follow-up:** row 300 (the three runtime defects and two compiler rules), then set `diffsParked` and `visualDiffParked` to false, restore the tests that require a finished render (`TestParkedVisualDiffIsSkipped_STU_100` becomes the gate's test again), and set `GOV-023` back to `DONE`.

## Options in detail

### Option 1

Correct and complete, and needed to un-park: about an hour of fixes, then a new release request and a render (about 45 minutes). Rejected for now: the maintainer wants the release flow usable first.

### Option 2

Unblocks Apply Register only; the next app with such a page is blocked again, and the platform defects stay hidden.

### Option 3

Unblocks every release at once, keeps the JSON diff, and is reversible in two constants. It weakens the evidence approvers see, which is recorded here and in the status of `GOV-023`.
