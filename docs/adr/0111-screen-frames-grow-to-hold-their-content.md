<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0111. A screen's frame grows to hold its page's scroll content

- **Status:** Accepted (maintainer, 2026-11-24)
- **Date:** 2026-11-24
- **Requirements:** `STU-056`, `STU-050`
- **Revises:** [ADR-0013](0013-plux-canvas.md) (how a screen's frame is sized)

## Context and problem

`STU-056` asked for the content past a screen's viewport to be shown as a translucent overflow region below the
device frame. The plugin canvas laid every page out at the device's height and drew the rest of its scroll view's
content below the frame. Two things went wrong while the maintainer built a real app (build log row 119):

- A page with bars at its foot — persistent footer buttons, a navigation bar, a bottom sheet — had them drawn at the
  device's foot, over the content that scrolls under them, and the content past the device continued below the frame.
  The filters sheet's "Show results" button covered its category chips; on a device nothing overlaps.
- A scroll view wrapped in a `SafeArea` (or a `FormScope`) — 47 of the app's screens — was laid out in the device's
  height and faded at its foot, so its content below the fold could be neither seen nor edited on the canvas.

The maintainer decided: not a clip — a screen's frame grows to hold all of its content, so what sits above a footer
can be seen and worked on; frames start at the device's height and grow as content is added; screens of different
heights on the canvas are fine.

## Decision drivers

- Everything a page shows on a device, after scrolling, can be seen and edited on the canvas.
- The bars stand where a device shows them relative to the content: below it, never over it.
- No cost to the canvas's performance (spec §1.4, `STU-120`).
- What a device would cut off still reads as a problem, not as a longer page.

## Considered options

1. Keep the overflow region; clip the body at the bars' top and draw the rest below the frame, moved down.
2. Grow the frame to hold the page's own scroll content, the bars at the grown frame's foot.

## Decision

Chosen option: **2**, as the maintainer decided.

- A design screen's frame is as high as the device, or as high as its page needs to show all of its own scroll view's
  content, whichever is more. The page's own scroll view is the scaffold's body, or the screen's root, when it is a
  scroll view or one wrapped only in `SafeArea`, `Padding`, `FormScope`, `RefreshIndicator` or `Scrollbar`.
- The page's `Scaffold` is laid out at the device's height first and, when its body's scroll content is higher than
  the body, once more grown by the difference. The second pass gives the scroll content the constraints it already
  had, so it is not laid out again; the bars, the floating action button and the bottom sheet stand at the grown
  frame's foot. `MediaQuery`'s height stays the device's.
- A short mark outside each side of a grown frame shows where the device's screen ends before it scrolls; it covers
  none of the frame's content.
- Content that overflows a box that does not scroll — cut off on a device — is still drawn below the frame in the
  translucent overflow region.
- A live screen (`STU-068`) keeps the device's height and scrolls inside it. A page under a dialog or sheet page keeps
  the device's height: the route is laid out at the device's size.

## Consequences

- **Positive:** a page's whole content is on the canvas, in the order the device shows it, and can be selected,
  dropped into and edited; the bars never cover it. The frame shrinks back as content is removed.
- **Negative:** screens on one canvas have different heights. Saved screen places were chosen for the heights of the
  time; a grown screen can reach the one below it until **Arrange** is used.
- **Follow-up:** `STU-056` describes the grown frame and keeps the overflow region for content a device cuts off (spec
  1.22.0).
