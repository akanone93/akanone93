<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0110. The canvases have no alignment guides, distance labels or snapping

- **Status:** Accepted (maintainer, 2026-11-24)
- **Date:** 2026-11-24
- **Requirements:** `STU-032`, `STU-050`, `STU-067`
- **Revises:** [ADR-0013](0013-plux-canvas.md) (the interaction overlay's contents)

## Context and problem

`STU-032` asked for snapping and alignment guides when plugin boxes are dragged on the app canvas,
`STU-050` for snapping when screens are dragged on the plugin canvas, and `STU-067` for smart guides
and distance measurement when widgets are dragged inside a stack. All three were built: each drag
move compared the dragged box with every other box, held a snap per axis with hysteresis, merged the
aligned lines into guides and measured the gaps to the nearest neighbours, and the page's overlay
drew the lines and labels.

The maintainer reviewed the canvases while building a real app (build log row 225) and decided that
the guides are not a priority: the priority is the performance and usability of the canvas, and the
guides are to be removed completely. Snapping without its guides is invisible magnetism — a box that
jumps for no reason the person can see — so it goes with them.

## Decision drivers

- Performance of the canvas first (spec §1.4): no per-move work that grows with the number of boxes.
- Usability: a dragged box goes exactly where the pointer puts it.
- Less code to maintain on the two canvases and the overlay.

## Considered options

1. Keep guides and snapping as they are.
2. Keep snapping, drop the guides.
3. Remove guides, distance labels and snapping on both canvases.

## Decision

Chosen option: **3**, as the maintainer decided on 2026-11-24 ("remove them completely; the current
priority is the performance and usability of the canvas").

What stays:

- A dragged plugin box, screen, target box or stack child follows the pointer, rounded to whole
  world units, and is saved where it is dropped; the arrow keys still nudge by 1 (Shift: 10).
- The insertion indicator of a drop into a column, row or wrap, the drop target's outline, the
  marquee and the ghost box of a drag or resize. The canvas package's overlay draws one indicator bar
  (`indicatorBar`), never thinner than two device pixels.
- A resized width or height still snaps to a dimension token near it, and a spacing handle to a
  spacing token (PC-18): that is about the theme's tokens, not about other boxes, and it shows its
  value as it goes.

What goes: the per-axis snap to other boxes' edges and centres with its hysteresis, the merged
guide lines, the distance labels to the nearest siblings, and their styles.

## Consequences

- **Positive:** a drag move no longer scans every other box on the canvas; the dragged box moves
  exactly with the pointer; the overlay draws less.
- **Negative:** lining boxes up by eye is harder. **Arrange** on both canvases lays them out on a
  grid of layers, and **Move to…** places a plugin box at exact coordinates.
- **Follow-up:** `STU-032`, `STU-050` and `STU-067` no longer name snapping, alignment guides, smart
  guides or distance measurement (spec 1.21.0); the Studio guide no longer describes them.
