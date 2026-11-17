// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Box } from "./geometry.ts";

/** Where a drop lands: a list of `parent` at `index`, one of its slots, or a Stack position (PC-20). */
export interface DropTarget {
  readonly parent: string;
  readonly index: number;
  /** The insertion indicator: a line in canvas coordinates. */
  readonly indicator: Box;
  /** Set for a Stack: the drop point in the Stack's coordinates, rounded to 1 px (F5). */
  readonly position?: { readonly left: number; readonly top: number };
  /** Set for a drop into a slot (PC-20): an empty single slot it fills, or a list slot it goes into at `index`. */
  readonly slot?: string;
  /** Set when `slot` holds a list. */
  readonly list?: boolean;
  /** A widget the dropped one goes in first, as the place needs: a `SliverToBoxAdapter` among slivers. */
  readonly wrap?: string;
}
