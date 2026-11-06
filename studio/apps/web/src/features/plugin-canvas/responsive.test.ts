// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { atSizeClass, type WidgetNode } from "@plux/layout";
import { produce } from "immer";
import { resolveFrame } from "../preview/frames.ts";
import { defaultPreview, previewDevice, previewRender } from "../preview/switches.ts";
import { applyConditions, evaluateCondition } from "./conditions.ts";
import { withLocaleOverrides } from "./locale-overrides.ts";
import { emptyScope } from "./mock-bindings.ts";
import type { Node } from "./plugin-model.ts";
import { adaptiveNavigationRecipe, adaptiveState, deviceRoot, foldMarks, sizeMarks } from "./responsive.ts";

const look = (frame: string, orientation: "portrait" | "landscape" = "portrait") => {
  const f = resolveFrame(frame);
  return previewRender(defaultPreview, f.platform, previewDevice(f, orientation, true));
};

describe("PC-33 responsive design on the canvas [WGT-010]", () => {
  test("the device root a condition reads: the frame's size class and the preview's switches", () => {
    expect(deviceRoot(look("pixel"))).toMatchObject({ sizeClass: "compact", platform: "android", darkMode: false });
    expect(deviceRoot(look("ipad"))).toMatchObject({ sizeClass: "medium", platform: "ios" });
    expect(deviceRoot(look("desktop"))["sizeClass"]).toBe("expanded");
    const scope = { ...emptyScope, device: deviceRoot(look("ipad")) };
    expect(evaluateCondition("device.sizeClass == 'medium'", new Map(), {}, scope)).toBe(true);
    expect(evaluateCondition('device.sizeClass != "medium"', new Map(), {}, scope)).toBe(false);
    expect(evaluateCondition("page.mode == 'edit'", new Map(), {}, { ...emptyScope, page: { mode: "edit" } })).toBe(
      true,
    );
  });

  test("a visibility on the size class is left to the layout; an If on it becomes a Responsive", () => {
    const root = {
      id: "col",
      type: "Column",
      children: [
        { id: "wide", type: "Text", visible: { $expr: "device.sizeClass != 'compact'" } },
        { id: "dark", type: "Text", visible: { $expr: "device.darkMode" } },
        {
          id: "if",
          type: "If",
          props: { condition: { $expr: "device.sizeClass == 'expanded'" } },
          // biome-ignore lint/suspicious/noThenProperty: an If's slot is named `then` in the widget registry
          slots: { then: { id: "a", type: "Text" }, else: { id: "b", type: "Text" } },
        },
      ],
    } as unknown as Node;
    const scope = { ...emptyScope, device: deviceRoot(look("pixel")) };
    const drawn = applyConditions(root, new Map(), {}, scope) as unknown as {
      children: { id: string; onlyIn?: string[]; type: string; slots?: Record<string, { id: string }> }[];
    };
    expect(drawn.children.map((c) => c.id)).toEqual(["wide", "if"]);
    expect(drawn.children[0]?.onlyIn).toEqual(["medium", "expanded"]);
    const responsive = drawn.children[1];
    expect(responsive?.type).toBe("Responsive");
    expect(Object.fromEntries(Object.entries(responsive?.slots ?? {}).map(([k, v]) => [k, v.id]))).toEqual({
      compact: "b",
      medium: "b",
      expanded: "a",
    });
    const at = (w: number) => (atSizeClass(drawn as unknown as WidgetNode, w)?.children ?? []).map((c) => c.id);
    expect(at(400)).toEqual(["if"]);
    expect(at(700)).toEqual(["wide", "if"]);
  });

  test("a locale's override keeps its value in every size class that sets the same prop", () => {
    const node = {
      id: "n",
      type: "SizedBox",
      props: { width: 100 },
      responsive: { medium: { width: 200, height: 40 } },
      localeOverrides: { de: { width: 160 } },
    };
    const out = withLocaleOverrides(node, "de-DE");
    expect(out["props"]).toEqual({ width: 160 });
    expect(out["responsive"]).toEqual({ medium: { width: 160, height: 40 } });
  });

  test("marks the layers that apply in the frame's size class and the widgets shown only somewhere", () => {
    const root = {
      id: "col",
      type: "Column",
      children: [
        { id: "a", type: "Padding", responsive: { medium: { padding: 24 }, expanded: { padding: 32, width: 9 } } },
        { id: "b", type: "Text", onlyIn: ["medium", "expanded"] },
        { id: "c", type: "Text", onlyIn: ["compact"] },
      ],
    };
    expect([...sizeMarks([root], "compact")]).toEqual([["c", { kind: "only", classes: ["compact"] }]]);
    expect([...sizeMarks([root], "expanded")]).toEqual([
      ["a", { kind: "layer", layer: "expanded", props: ["padding", "width"] }],
      ["b", { kind: "only", classes: ["medium", "expanded"] }],
    ]);
  });

  test("on a foldable, controls and images across the fold are marked; texts and narrow widgets are not", () => {
    const screen = { l: 0, t: 0, r: 600, b: 800 };
    const boxes = new Map([
      ["btn", { l: 200, t: 100, r: 400, b: 140 }],
      ["img", { l: 0, t: 300, r: 600, b: 500 }],
      ["txt", { l: 0, t: 500, r: 600, b: 520 }],
      ["left", { l: 20, t: 600, r: 290, b: 640 }],
    ]);
    const types = new Map([
      ["btn", "FilledButton"],
      ["img", "Image"],
      ["txt", "Text"],
      ["left", "FilledButton"],
    ]);
    expect([...foldMarks(boxes, types, () => screen, "vertical")].sort()).toEqual(["btn", "img"]);
    expect([...foldMarks(boxes, types, () => screen, "horizontal")]).toEqual(["img"]);
    expect(look("foldable").device.fold).toBe("vertical");
    expect(look("foldable", "landscape").device.fold).toBe("horizontal");
    expect(look("pixel").device.fold).toBeUndefined();
  });

  test("a page's navigation bar becomes adaptive: the bar on compact screens, a rail beside the body from 600 dp", () => {
    const destinations = [{ icon: { name: "home" }, label: "Home" }];
    const bar = {
      id: "bar",
      type: "NavigationBar",
      props: { destinations, selectedIndex: 0 },
      events: { onDestinationSelected: { steps: [{ id: "go", action: "navigate", input: { route: "home" } }] } },
    };
    const page = { id: "s", type: "Scaffold", slots: { body: { id: "body", type: "Text" }, bottomNavigationBar: bar } };
    const fresh = structuredClone(page);
    expect(adaptiveState(page, "bar")).toBe("ready");
    let n = 0;
    adaptiveNavigationRecipe("bar", () => `n${++n}`)(page);
    expect(adaptiveState(page, "bar")).toBe("adaptive");
    const body = page.slots.body as unknown as { type: string; children: Record<string, unknown>[] };
    expect(body.type).toBe("Row");
    expect(body.children.map((c) => c["type"])).toEqual(["NavigationRail", "VerticalDivider", "Expanded"]);
    const rail = body.children[0] as Record<string, unknown>;
    expect(rail["props"]).toEqual({ destinations, selectedIndex: 0, labelType: "all" });
    expect(rail["events"]).toEqual(bar.events);
    expect(rail["responsive"]).toEqual({ expanded: { extended: true, labelType: "none" } });
    expect(rail["visible"]).toEqual({ $expr: "device.sizeClass != 'compact'" });
    expect((page.slots.bottomNavigationBar as Record<string, unknown>)["visible"]).toEqual({
      $expr: "device.sizeClass == 'compact'",
    });
    const filled = body.children[2] as { slots: { child: { id: string } } };
    expect(filled.slots.child.id).toBe("body");
    // The editor runs recipes on an immer draft: the rail's copies are plain values, not draft proxies.
    const drafted = produce({ root: structuredClone(fresh) }, (d) => {
      adaptiveNavigationRecipe("bar", () => `d${++n}`)(d.root);
    });
    expect(adaptiveState(drafted.root, "bar")).toBe("adaptive");
    adaptiveNavigationRecipe("bar", () => "again")(page);
    expect((page.slots.body as unknown as { children: unknown[] }).children).toHaveLength(3);
    expect(adaptiveState({ id: "x", type: "Column" }, "bar")).toBe("none");
  });
});
