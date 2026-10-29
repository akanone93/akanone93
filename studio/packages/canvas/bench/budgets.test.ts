// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { checks, FRAME_BUDGET_MS, GAP_BUDGET_MS, SETTLE_BUDGET_MS, type Summary, violations } from "./budgets.ts";

const pan = { p95: 16.67, max: 16.67, settleVisibleMs: 45 };
const drag = { p95: 16.67, max: 33.33, dragStartMs: 150, dropSettleMs: 250 };
const ok: Summary = {
  sceneA: { pan, drag },
  sceneB: { pan },
  sceneC: { pan, drag },
  propertyToCanvas: { p95: 19 },
};

describe("[STU-120] [STU-121] M-13 canvas budgets of the benchmark", () => {
  test("a run inside every budget has no violation", () => {
    expect(violations(ok)).toEqual([]);
  });

  test("holds the frame, gap and settle limits of the specification", () => {
    expect(FRAME_BUDGET_MS).toBe(16.7);
    expect(GAP_BUDGET_MS).toBe(50);
    expect(SETTLE_BUDGET_MS).toBe(300);
    expect(checks(ok)).toHaveLength(17);
  });

  test("names each budget a number exceeds", () => {
    const slow: Summary = {
      ...ok,
      sceneA: { pan: { ...pan, p95: 33.34 }, drag: { ...drag, dropSettleMs: 301 } },
      propertyToCanvas: { p95: 50.1 },
    };
    const missed = violations(slow);
    expect(missed).toHaveLength(3);
    expect(missed[0]).toContain("scene A pan p95");
    expect(missed[1]).toContain("scene A drop settle");
    expect(missed[2]).toContain("property change p95");
  });

  test("a limit itself is within budget", () => {
    expect(violations({ ...ok, propertyToCanvas: { p95: 50 } })).toEqual([]);
  });

  test("a number the run did not report counts as missed", () => {
    expect(violations({})).toHaveLength(17);
    expect(violations({ ...ok, sceneB: {} })[0]).toContain("not reported");
  });
});
