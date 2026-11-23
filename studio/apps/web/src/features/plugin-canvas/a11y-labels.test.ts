// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { badgeAt, keepApart, type Label, tagAbove, tagBelow } from "./a11y-labels.ts";

const view = { width: 1000, height: 800 };

describe("[PC-31] the Accessibility view's labels never cover one another (#219)", () => {
  test("a label overlapping one placed before it gives way; the first ones listed win", () => {
    const labels: Label<string>[] = [
      { rect: tagBelow(100, 100, "31 × 22, under 48"), mark: "target a" },
      { rect: tagBelow(110, 104, "31 × 22, under 48"), mark: "target b" },
      { rect: badgeAt(105, 110), mark: "order 1" },
      { rect: badgeAt(400, 400), mark: "order 2" },
    ];
    expect(keepApart(labels, view).map((l) => l.mark)).toEqual(["target a", "order 2"]);
  });

  test("labels side by side all stay, and a label outside the view is left out", () => {
    const row = Array.from({ length: 20 }, (_, i) => ({ rect: badgeAt(20 + i * 20, 20), mark: i }));
    expect(keepApart(row, view)).toHaveLength(20);
    expect(keepApart([{ rect: badgeAt(-50, 20), mark: 0 }], view)).toHaveLength(0);
  });

  test("a contrast tag ends at the text's top end and stands above it", () => {
    const r = tagAbove(300, 200, "2.1:1, needs 4.5");
    expect(r.r).toBe(300);
    expect(r.b).toBeLessThanOrEqual(200);
  });

  test("a crowded view keeps a readable few: none of the kept labels overlaps another", () => {
    const many = Array.from({ length: 5000 }, (_, i) => ({ rect: badgeAt((i * 37) % 1000, (i * 53) % 800), mark: i }));
    const kept = keepApart(many, view);
    expect(kept.length).toBeGreaterThan(100);
    expect(kept.length).toBeLessThan(5000);
    for (const a of kept) {
      for (const b of kept) {
        const apart =
          a === b || a.rect.r <= b.rect.l || b.rect.r <= a.rect.l || a.rect.b <= b.rect.t || b.rect.b <= a.rect.t;
        expect(apart).toBe(true);
      }
    }
  });
});
