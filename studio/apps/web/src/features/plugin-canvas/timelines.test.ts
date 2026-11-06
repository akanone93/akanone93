// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  addTimelineRecipe,
  applyTimeline,
  currentValue,
  drivenMs,
  lerpValue,
  playedMs,
  putKeyframeRecipe,
  removeKeyframeRecipe,
  removeTimelineRecipe,
  scrollViewsOf,
  setTimelineRecipe,
  type Timeline,
  timelinesOf,
  trackValue,
} from "./timelines.ts";

const fade: Timeline = {
  id: "t",
  name: "intro",
  durationMs: 1000,
  tracks: [
    {
      node: "box",
      prop: "opacity",
      keyframes: [
        { atMs: 0, value: 0 },
        { atMs: 500, value: 1, curve: "linear" },
      ],
    },
    {
      node: "box",
      prop: "color",
      keyframes: [
        { atMs: 0, value: "#000000FF" },
        { atMs: 1000, value: "#FF0000FF" },
      ],
    },
  ],
};

describe("PC-30 timelines on the canvas [ANI-002] [ANI-006] [STU-068]", () => {
  test("a track's value eases between keyframes; numbers, colours and offsets blend", () => {
    const [opacity, color] = fade.tracks;
    expect(opacity && trackValue(opacity, -10)).toBe(0);
    expect(opacity && trackValue(opacity, 250)).toBeCloseTo(0.5, 5);
    expect(opacity && trackValue(opacity, 900)).toBe(1);
    expect(color && trackValue(color, 500)).toBe("#800000FF");
    expect(lerpValue({ dx: 0, dy: 10 }, { dx: 20, dy: 10 }, 0.5)).toEqual({ dx: 10, dy: 10 });
    expect(lerpValue({ $token: "a" }, { $token: "b" }, 0.7)).toEqual({ $token: "b" });
  });

  test("the page is drawn at the playhead; a driven timeline follows the scroll; repeats and reverse play", () => {
    const root = { id: "col", type: "Column", children: [{ id: "box", type: "Opacity", props: { opacity: 1 } }] };
    const drawn = applyTimeline(root, fade, 250) as { children: { props: Record<string, unknown> }[] };
    expect(drawn.children[0]?.props["opacity"]).toBeCloseTo(0.5, 5);
    expect(root.children[0]?.props.opacity).toBe(1);
    expect(drivenMs({ ...fade, driver: { kind: "scroll", node: "list", extent: 400 } }, 100)).toBe(250);
    expect(playedMs(fade, 1500)).toBeUndefined();
    expect(playedMs({ ...fade, repeat: 1, reverse: true }, 1250)).toBe(750);
    expect(playedMs({ ...fade, repeatForever: true }, 5300)).toBe(300);
    expect(
      scrollViewsOf({ id: "r", type: "Scaffold", slots: { body: { id: "list12345", type: "ListView" } } }),
    ).toEqual([{ id: "list12345", name: "ListView · 2345" }]);
  });

  test("timelines and keyframes are edited in the page; a new track starts from the current value", () => {
    const page: Record<string, unknown> = { kind: "page" };
    addTimelineRecipe("t1", "intro")(page);
    setTimelineRecipe("t1", "durationMs", 800)(page);
    setTimelineRecipe("t1", "reverse", true)(page);
    const root = { id: "box", type: "Opacity", props: { opacity: 0.2 } };
    putKeyframeRecipe("t1", "box", "opacity", 400, 1, currentValue(root, "box", "opacity"))(page);
    const [t] = timelinesOf(page);
    expect(t).toMatchObject({ durationMs: 800, reverse: true });
    expect(t?.tracks[0]?.keyframes).toEqual([
      { atMs: 0, value: 0.2 },
      { atMs: 400, value: 1 },
    ]);
    putKeyframeRecipe("t1", "box", "opacity", 400, 0.8, 0)(page);
    expect(timelinesOf(page)[0]?.tracks[0]?.keyframes[1]).toEqual({ atMs: 400, value: 0.8 });
    removeKeyframeRecipe("t1", "box", "opacity", 400)(page);
    expect(timelinesOf(page)[0]?.tracks).toEqual([]);
    setTimelineRecipe("t1", "reverse", false)(page);
    expect(timelinesOf(page)[0]?.reverse).toBeUndefined();
    removeTimelineRecipe("t1")(page);
    expect("animations" in page).toBe(false);
  });
});
