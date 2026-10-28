// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// The canvas budgets the benchmark enforces on the median of its runs (QA-007): a number over its
// budget fails `make studio-bench-canvas`.
//   STU-045, STU-120, NFR-031: a pan or drag frame fits 60 Hz (p95 of the rAF interval ≤ 16.7 ms).
//   ADR-0013 M-13: the longest gap between frames ≤ 50 ms; every tile in the viewport at full
//     resolution within 300 ms after a gesture ends (settle).
//   STU-121, NFR-032: a property change reaches the canvas frame in ≤ 50 ms at p95
//     (PROPERTY_BUDGET_MS, property.ts).

import { PROPERTY_BUDGET_MS } from "./property.ts";

/** 60 Hz frame budget, ms. */
export const FRAME_BUDGET_MS = 16.7;
/** Longest gap between frames, ms (M-13). */
export const GAP_BUDGET_MS = 50;
/** Settle after a gesture ends, ms (M-13). */
export const SETTLE_BUDGET_MS = 300;

type Num = number | undefined;
type Pan = { p95?: Num; max?: Num; settleVisibleMs?: Num };
type Drag = { p95?: Num; max?: Num; dragStartMs?: Num; dropSettleMs?: Num };

/** The numbers of the benchmark's median that carry a budget. */
export type Summary = {
  sceneA?: { pan?: Pan; drag?: Drag };
  sceneB?: { pan?: Pan };
  sceneC?: { pan?: Pan; drag?: Drag };
  propertyToCanvas?: { p95?: Num };
};

type Check = { name: string; value: Num; limit: number };

/** Every budget the summary is held to, with the measured value (undefined when the run did not report it). */
export function checks(summary: Summary): Check[] {
  const out: Check[] = [];
  const pan = (scene: string, p: Pan | undefined): void => {
    out.push({ name: `${scene} pan p95 (STU-120)`, value: p?.p95, limit: FRAME_BUDGET_MS });
    out.push({ name: `${scene} pan longest gap (M-13)`, value: p?.max, limit: GAP_BUDGET_MS });
    out.push({ name: `${scene} pan settle, visible (M-13)`, value: p?.settleVisibleMs, limit: SETTLE_BUDGET_MS });
  };
  const drag = (scene: string, d: Drag | undefined, start: boolean): void => {
    out.push({ name: `${scene} drag p95 (STU-045)`, value: d?.p95, limit: FRAME_BUDGET_MS });
    out.push({ name: `${scene} drag longest gap (M-13)`, value: d?.max, limit: GAP_BUDGET_MS });
    out.push({ name: `${scene} drop settle (M-13)`, value: d?.dropSettleMs, limit: SETTLE_BUDGET_MS });
    if (start) {
      out.push({ name: `${scene} drag start settle (M-13)`, value: d?.dragStartMs, limit: SETTLE_BUDGET_MS });
    }
  };
  pan("scene A", summary.sceneA?.pan);
  drag("scene A", summary.sceneA?.drag, false);
  pan("scene B", summary.sceneB?.pan);
  pan("scene C", summary.sceneC?.pan);
  drag("scene C", summary.sceneC?.drag, true);
  out.push({ name: "property change p95 (STU-121)", value: summary.propertyToCanvas?.p95, limit: PROPERTY_BUDGET_MS });
  return out;
}

/** The budgets the summary misses, as readable lines; a number the run did not report counts as missed. */
export function violations(summary: Summary): string[] {
  return checks(summary).flatMap((c) =>
    c.value === undefined
      ? [`${c.name}: not reported`]
      : c.value > c.limit
        ? [`${c.name}: ${c.value} ms exceeds ${c.limit} ms`]
        : [],
  );
}
