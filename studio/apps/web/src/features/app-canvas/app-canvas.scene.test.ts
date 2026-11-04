// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { I18n } from "@lingui/core";
import type { PluginSummary } from "@plux/api";
import {
  ArrowCanvas,
  ArrowRouter,
  buildSceneModel,
  deriveAppBundles,
  type ItemSpec,
  Scene,
  SOFT_SHADOW_MARGIN,
  Viewport,
} from "@plux/canvas";
import type { LoadedDoc } from "../safety/index.ts";
import { appCanvasMessages } from "./app-canvas.messages.ts";
import { BOX } from "./app-canvas.model.ts";
import {
  appGraph,
  badgesOf,
  boxDepth,
  boxItem,
  boxItems,
  boxLayers,
  boxOfHits,
  FOCUS_ITEM,
  focusItem,
  frameItems,
  layoutPills,
  nativeBox,
  redirectLink,
  removeLink,
  type SceneBox,
} from "./app-canvas.scene.ts";

const i18n = { _: (d: { message: string }) => d.message } as unknown as I18n;
const m = appCanvasMessages(i18n);

const summary = (over: Record<string, unknown>) =>
  ({
    plugin: { id: "loans", key: "loans", name: "Loans", archived: false },
    lockHolder: "Ada",
    unpublishedChanges: true,
    validationErrors: 2n,
    killed: true,
    ...over,
  }) as unknown as PluginSummary;

const box = (id: string, x: number, y: number, s?: PluginSummary): SceneBox => ({
  id,
  key: id,
  x,
  y,
  title: id,
  native: false,
  archived: false,
  badges: badgesOf(s, id === "loans", m),
});

/** The scene model of `items` (studio-development §9.2): what the worker would hold. */
function model(items: readonly ItemSpec[]) {
  const scene = new Scene();
  scene.apply(items.map((item) => ({ op: "add" as const, item })));
  return buildSceneModel(scene, new Viewport(800, 600));
}

describe("C1 scene items [STU-030]", () => {
  test("a box is a labelled card with a soft shadow, its mark, title, key and every badge as a pill inside it", () => {
    const b = box("loans", 100, 40, summary({}));
    const nodes = model(boxItems(b)).nodes;
    const main = nodes.find((n) => n.id === boxItem("loans"));
    expect(main).toMatchObject({
      kind: "rect",
      label: "loans",
      // The card's bounds take in its shadow: a 1-unit stroke and the shadow's reach on every side.
      bounds: {
        x: 99.5 - SOFT_SHADOW_MARGIN,
        y: 39.5 - SOFT_SHADOW_MARGIN,
        w: BOX.width + 1 + 2 * SOFT_SHADOW_MARGIN,
        h: BOX.height + 1 + 2 * SOFT_SHADOW_MARGIN,
      },
    });
    const texts = nodes.filter((n) => n.kind === "text").map((n) => n.label);
    // Title and key, the initial on its tile, then the pills; what does not fit in the status row is counted in a "+N" pill.
    expect(texts.slice(0, 3)).toEqual(["loans", "loans", "L"]);
    const pills = texts.slice(3);
    const more = pills.find((t) => t.startsWith("+"));
    const hidden = more === undefined ? 0 : Number(more.slice(1));
    expect(pills.length - (more === undefined ? 0 : 1) + hidden).toBe(b.badges.length);
    expect(pills[0]).toBe(m.entryBadge);
    for (const n of nodes.filter((x) => x.id !== boxItem("loans"))) {
      // Every part within the box, its 1-unit stroke included.
      expect(n.bounds.x).toBeGreaterThanOrEqual(99.5);
      expect(n.bounds.x + n.bounds.w).toBeLessThanOrEqual(100.5 + BOX.width);
      expect(n.bounds.y + n.bounds.h).toBeLessThanOrEqual(40.5 + BOX.height);
      expect(boxOfHits([n.id])).toBe("loans");
    }
  });

  test("a valid plugin carries the success badge; frames and the focus ring are outside the boxes' hits", () => {
    expect(
      badgesOf(summary({ lockHolder: "", unpublishedChanges: false, validationErrors: 0n, killed: false }), false, m),
    ).toEqual([{ status: "success", label: m.valid }]);
    const items = [
      ...frameItems({ id: "f1", label: "Onboarding", x: 0, y: 0, width: 600, height: 300 }),
      focusItem(box("cards", 10, 10)),
    ];
    expect(model(items).nodes.map((n) => n.label)).toContain("Onboarding");
    expect(boxOfHits(items.map((i) => i.id))).toBeUndefined();
    expect(items.at(-1)?.id).toBe(FOCUS_ITEM);
  });
});

const page = (pluginId: string, id: string, steps: object[] = []): LoadedDoc => ({
  pluginId,
  path: `pages/${id}.page.json`,
  kind: "page",
  revision: 3,
  json: { id, key: id, route: id, title: id, kind: "page", root: { id: `${id}-r`, events: { onPressed: { steps } } } },
});
const nav = (route: string) => ({ action: "navigate", input: { route } });
const docs: LoadedDoc[] = [
  page("loans", "home", [nav("offers"), nav("terms"), nav("camera"), nav("nowhere")]),
  page("cards", "offers"),
  page("cards", "terms", [nav("home")]),
];

describe("box depth [STU-032]", () => {
  test("each box draws in its own band, later boxes over earlier ones, all under the expanded bundle", () => {
    const a = boxItems(box("a", 0, 0), undefined, false, boxDepth(0, 3));
    const b = boxItems(box("b", 50, 50), undefined, true, boxDepth(2, 3));
    const top = (items: readonly ItemSpec[]) => Math.max(...items.map((i) => i.z));
    const bottom = (items: readonly ItemSpec[]) => Math.min(...items.map((i) => i.z));
    expect(bottom(b)).toBeGreaterThan(top(a));
    expect(top(b)).toBeLessThan(11);
    expect(top(boxItems(box("z", 0, 0), undefined, false, boxDepth(99, 100)))).toBeLessThan(11);
  });
});

describe("box bands [STU-032] [STU-120]", () => {
  test("boxes that overlap nothing share the lowest band; a later box over others goes above them", () => {
    const apart = [box("a", 0, 0), box("b", 300, 0), box("c", 600, 0)];
    expect(boxLayers(apart)).toEqual([0, 0, 0]);
    // c is dragged onto a and b, which overlap each other too.
    const piled = [box("a", 0, 0), box("b", 100, 50), box("c", 50, 20)];
    expect(boxLayers(piled)).toEqual([0, 1, 2]);
  });
});

describe("badge pills [STU-030]", () => {
  test("short badges share a row; too many end in a +N pill within the rows asked for", () => {
    const few = layoutPills(
      [
        { status: "neutral", label: "Entry" },
        { status: "success", label: "Valid" },
      ],
      200,
    );
    expect(few.map((p) => [p.label, p.row])).toEqual([
      ["Entry", 0],
      ["Valid", 0],
    ]);
    const many = layoutPills(
      Array.from({ length: 8 }, (_, i) => ({ status: "warning" as const, label: `Long badge number ${i}` })),
      200,
    );
    expect(many.at(-1)?.label).toBe(`+${8 - (many.length - 1)}`);
    expect(Math.max(...many.map((p) => p.row))).toBe(0);
    expect(many.every((p) => p.x + p.w <= 200)).toBe(true);
    const three = layoutPills(
      Array.from({ length: 8 }, (_, i) => ({ status: "warning" as const, label: `Long badge number ${i}` })),
      200,
      3,
    );
    expect(Math.max(...three.map((p) => p.row))).toBe(2);
    expect(three.at(-1)?.label.startsWith("+")).toBe(true);
  });
});

describe("C1 arrows [STU-040] [STU-041]", () => {
  const graph = appGraph(docs, ["camera"]);

  test("links between plugins and to native routes are lanes; unknown routes are not [STU-033]", () => {
    expect(graph.links.map((l) => l.edge.title)).toEqual(
      ["home → offers", "home → terms", "home → camera", "terms → home"].sort(
        (a, b) => graph.links.findIndex((l) => l.edge.title === a) - graph.links.findIndex((l) => l.edge.title === b),
      ),
    );
    expect(graph.natives).toEqual(["camera"]);
    expect(graph.links.find((l) => l.route === "camera")?.edge.target).toBe(nativeBox("camera"));
    expect(graph.links.find((l) => l.route === "offers")?.edge.trigger).toBe("home · onPressed");
  });

  test("lanes bundle per plugin pair and are drawn as arrow items between the boxes", () => {
    const bundles = deriveAppBundles(
      graph.links.map((l) => l.edge),
      graph.boxOf,
    );
    expect(bundles.map((b) => [b.id, b.links.length])).toEqual([
      ["cards→loans", 1],
      ["loans→cards", 2],
      [`loans→${nativeBox("camera")}`, 1],
    ]);
    const router = new ArrowRouter([], []);
    const arrows = new ArrowCanvas(router);
    router.setGraph(
      [box("loans", 0, 0), box("cards", 600, 0), box(nativeBox("camera"), 0, 400)].map((b) => ({
        id: b.id,
        x: b.x,
        y: b.y,
        w: BOX.width,
        h: BOX.height,
      })),
      bundles,
    );
    const scene = new Scene();
    scene.apply(arrows.render());
    const ids = buildSceneModel(scene, new Viewport(800, 600)).nodes.map((n) => n.id);
    expect(ids.some((id) => id.startsWith("arrow:loans→cards:"))).toBe(true);
    expect(ids.some((id) => id.startsWith(`arrow:loans→${nativeBox("camera")}:`))).toBe(true);
    expect(boxOfHits(ids)).toBeUndefined();
  });

  test("redirect and remove edit exactly the one step of the lane [STU-046]", () => {
    const link = graph.links.find((l) => l.route === "terms");
    expect(link).toBeDefined();
    if (link === undefined) {
      return;
    }
    const redirect = redirectLink(docs, link, "offers");
    expect(redirect).toEqual([
      {
        pluginId: "loans",
        edits: [
          {
            path: "pages/home.page.json",
            ifRevision: 3,
            patch: [
              { op: "test", path: "/root/events/onPressed/steps/1/input/route", value: "terms" },
              { op: "replace", path: "/root/events/onPressed/steps/1/input/route", value: "offers" },
            ],
          },
        ],
      },
    ]);
    const remove = removeLink(docs, link);
    const edit = remove[0]?.edits[0];
    expect(edit !== undefined && "patch" in edit ? edit.patch.at(-1) : undefined).toEqual({
      op: "remove",
      path: "/root/events/onPressed/steps/1",
    });
  });
});
