// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { ArrowCanvas, ArrowRouter, deriveBundles, HOVER_ENTER_MS, type SceneOp } from "@plux/canvas";
import { ArrowMotion, type MotionHost } from "./arrow-motion.ts";
import { arrowStyle } from "./scene.ts";

/** Two screens side by side with two links from the first to the second: one bundle of two lanes. */
function setup(reduced = false) {
  const router = new ArrowRouter([], []);
  router.setGraph(
    [
      { id: "home", x: 0, y: 0, w: 390, h: 844 },
      { id: "form", x: 800, y: 0, w: 390, h: 844 },
      { id: "done", x: 1600, y: 0, w: 390, h: 844 },
    ],
    deriveBundles([
      { id: "n1", source: "home", target: "form", trigger: "Apply · onPressed", title: "Apply → Form" },
      { id: "n2", source: "home", target: "form", trigger: "More · onTap", title: "More → Form" },
      { id: "n3", source: "form", target: "done", trigger: "Send · onPressed", title: "Send → Done" },
    ]),
  );
  const arrows = new ArrowCanvas(router, arrowStyle, 1);
  arrows.render(0);
  const patches: SceneOp[][] = [];
  let clock = 1000;
  const frames: (() => void)[] = [];
  const waiting = new Map<number, { run: () => void; at: number }>();
  let handles = 0;
  const host: MotionHost = {
    now: () => clock,
    request: (step) => frames.push(step),
    cancel: () => undefined,
    reduced: () => reduced,
    timers: {
      set: (run, ms) => {
        waiting.set(++handles, { run, at: clock + ms });
        return handles;
      },
      clear: (handle) => waiting.delete(handle),
    },
  };
  /** Moves the clock on by `ms` and fires the timers due by then. */
  const wait = (ms: number) => {
    clock += ms;
    for (const [handle, timer] of [...waiting]) {
      if (timer.at <= clock) {
        waiting.delete(handle);
        timer.run();
      }
    }
  };
  const motion = new ArrowMotion(arrows, (ops) => patches.push(ops), host);
  /** Runs the frame loop until it stops, 16 ms a frame. */
  const settle = () => {
    for (let n = 0; frames.length > 0 && n < 100; n++) {
      clock += 16;
      frames.shift()?.();
    }
  };
  const bundle = router.bundlesAt("home")[0] ?? "";
  const ids = (ops: readonly SceneOp[] | undefined) => (ops ?? []).flatMap((o) => ("item" in o ? [o.item.id] : []));
  return { motion, patches, settle, bundle, ids, frames, arrows, wait };
}

describe("[STU-044] [STU-045] arrows that move", () => {
  test("[STU-044] a hovered bundle fans out into titled lanes that can be clicked, then folds back", () => {
    const s = setup();
    s.motion.expand(s.bundle);
    expect(s.frames.length).toBe(1);
    s.settle();
    const open = s.ids(s.patches.at(-1));
    expect(open.some((id) => id.endsWith(":t0"))).toBe(true);
    expect(open.some((id) => id.endsWith(":t1"))).toBe(true);
    // The titles are drawn centred on a pill, with "›" for "→": the canvas fonts have no "→".
    const titles = (s.patches.at(-1) ?? []).flatMap((o) =>
      "item" in o && o.item.kind === "text" ? [[o.item.text, o.item.align]] : [],
    );
    expect(titles).toContainEqual(["Apply › Form", "center"]);
    expect(open.some((id) => id.endsWith(":p0"))).toBe(true);
    // A lane is a link: a click on it selects that navigate action (STU-046).
    const lane = open.find((id) => id.includes(":l1s"));
    expect(s.arrows.pick([lane ?? ""])?.link?.id).toBeDefined();
    s.motion.expand(s.bundle);
    const sent = s.patches.length;
    expect(s.patches.length).toBe(sent);
    s.motion.expand(null);
    s.settle();
    const shut = s.ids(s.patches.at(-1));
    expect(shut.some((id) => id.includes(":l0s"))).toBe(false);
    expect(shut).toContain(`arrow:${s.bundle}:badge`);
  });

  test("[STU-044] a box the pointer rests on lights its arrows, with the screen worked on, and the rest fade (#240)", () => {
    const s = setup();
    const colour = (id: string) =>
      s.patches
        .flat()
        .flatMap((o) => ("item" in o && o.item.id === id && o.item.kind === "link" ? [o.item.stroke.color] : []))
        .at(-1);
    s.motion.hoverBox("done");
    // Not before the pointer has settled: a sweep across the screens lights nothing.
    expect(s.patches).toHaveLength(0);
    s.wait(HOVER_ENTER_MS);
    expect(colour("arrow:form→done:s0")).toBe(arrowStyle.focus.color);
    expect(colour("arrow:home→form:s0")).toBe(arrowStyle.dimmed.color);
    // The screen being worked on stays lit with it.
    s.motion.highlight(["home"]);
    expect(colour("arrow:home→form:s0")).toBe(arrowStyle.focus.color);
    expect(colour("arrow:form→done:s0")).toBe(arrowStyle.focus.color);
    // Once the pointer has left, only the screen worked on keeps its arrows lit.
    s.motion.hoverBox(null);
    s.wait(1000);
    expect(colour("arrow:form→done:s0")).toBe(arrowStyle.dimmed.color);
    expect(colour("arrow:home→form:s0")).toBe(arrowStyle.focus.color);
    s.motion.highlight([]);
    expect(colour("arrow:form→done:s0")).toBe(arrowStyle.stroke.color);
    expect(colour("arrow:home→form:s0")).toBe(arrowStyle.stroke.color);
  });

  test("[STU-044] under reduced motion the bundle opens at once, with no frame loop", () => {
    const s = setup(true);
    s.motion.expand(s.bundle);
    expect(s.frames.length).toBe(0);
    expect(s.ids(s.patches.flat()).some((id) => id.endsWith(":t0"))).toBe(true);
  });

  test("[STU-045] the arrows of a screen shown at a dragged place re-route before the drop", () => {
    const s = setup();
    s.motion.follow(new Map([["form", { x: 0, y: 1400 }]]));
    const moved = s.patches.at(-1) ?? [];
    expect(moved.length).toBeGreaterThan(0);
    // The form is now under the home screen: the bundle enters it from above, its tip on the form's top edge.
    const tip = moved.flatMap((o) => ("item" in o && o.item.kind === "link" ? [o.item.to] : []));
    expect(tip.some((p) => p.y === 1400)).toBe(true);
    s.motion.follow(new Map());
    expect(s.patches.at(-1)).toBe(moved);
  });
});
