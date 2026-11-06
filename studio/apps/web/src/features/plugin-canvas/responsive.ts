// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Responsive design on the canvas (PC-33, WGT-010): the `device` root a condition reads at design time, what the
 * Size classes view marks — the widgets a size-class layer changes in the frame's size class, the widgets shown only
 * in some size classes, and on a foldable the controls and images that cross the fold — and the recipe that makes a
 * page's navigation bar adaptive: a navigation rail beside the body from 600 dp, extended from 840 dp, as Material 3
 * lays navigation out by window size class.
 */

import { ACTIVE_LAYERS, type SizeClass, sizeClassOf } from "@plux/layout";
import { widget } from "@plux/schema";
import type { PreviewRender } from "../preview/switches.ts";
import type { Box } from "./select/geometry.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

const kidsOf = (n: Obj): unknown[] => [
  ...(isObj(n["slots"]) ? Object.values(n["slots"]).flatMap((v) => (Array.isArray(v) ? v : [v])) : []),
  ...(Array.isArray(n["children"]) ? n["children"] : []),
];

/** PXL's `device` root at design time (Appendix E.2): the frame's size class and the preview's switches. */
export function deviceRoot(look: PreviewRender): Obj {
  return {
    platform: look.platform === "cupertino" ? "ios" : "android",
    osVersion: "",
    locale: look.locale,
    textScale: look.textScale,
    darkMode: look.theme === "dark",
    sizeClass: sizeClassOf(look.device.width),
    textDirection: look.direction,
    calendar: "gregorian",
    assuranceLevel: "AL0",
    reduceMotion: false,
    boldText: false,
    highContrast: false,
    screenReader: false,
  };
}

/** What the Size classes view says of a widget. */
export type SizeMark =
  | { readonly kind: "layer"; readonly layer: "medium" | "expanded"; readonly props: readonly string[] }
  | { readonly kind: "only"; readonly classes: readonly SizeClass[] }
  | { readonly kind: "fold" };

/** The props a node's size-class layers set in `sc`, and the widest layer among them. */
function layerMark(n: Obj, sc: SizeClass): SizeMark | undefined {
  const responsive = isObj(n["responsive"]) ? n["responsive"] : {};
  const layers = ACTIVE_LAYERS[sc].filter((l) => isObj(responsive[l]) && Object.keys(responsive[l] as Obj).length > 0);
  const top = layers.at(-1);
  if (top === undefined) {
    return undefined;
  }
  const props = [...new Set(layers.flatMap((l) => Object.keys(responsive[l] as Obj)))].sort();
  return { kind: "layer", layer: top, props };
}

/** The size-class marks of the drawn screens in `sc`: layers that apply here, and widgets shown only somewhere. */
export function sizeMarks(roots: readonly unknown[], sc: SizeClass): Map<string, SizeMark> {
  const out = new Map<string, SizeMark>();
  const visit = (n: unknown) => {
    if (!isObj(n)) {
      return;
    }
    const id = typeof n["id"] === "string" ? n["id"] : undefined;
    const only = Array.isArray(n["onlyIn"]) ? (n["onlyIn"] as SizeClass[]) : undefined;
    const mark = only !== undefined && only.includes(sc) ? { kind: "only" as const, classes: only } : layerMark(n, sc);
    if (id !== undefined && mark !== undefined) {
      out.set(id, mark);
    }
    kidsOf(n).forEach(visit);
  };
  roots.forEach(visit);
  return out;
}

/** The widgets that must not sit across a fold: controls and images. */
const FOLD_SENSITIVE: ReadonlySet<string> = new Set(["Image", "Icon", "CircleAvatar", "TextField", "TextFormField"]);

const foldSensitive = (type: string): boolean =>
  FOLD_SENSITIVE.has(type) || widget(type)?.accessibility.interactive === true;

/**
 * The controls and images that cross a foldable's fold (PC-33): a vertical fold down the middle of a portrait screen,
 * a horizontal one across a landscape screen; `screens` gives each screen's box by its root's id.
 */
export function foldMarks(
  boxes: ReadonlyMap<string, Box>,
  types: ReadonlyMap<string, string>,
  screenOf: (id: string) => Box | undefined,
  fold: "vertical" | "horizontal",
): Set<string> {
  const out = new Set<string>();
  for (const [id, box] of boxes) {
    const screen = screenOf(id);
    if (screen === undefined || !foldSensitive(types.get(id) ?? "")) {
      continue;
    }
    const [lo, hi, mid] =
      fold === "vertical" ? [box.l, box.r, (screen.l + screen.r) / 2] : [box.t, box.b, (screen.t + screen.b) / 2];
    if (lo < mid - 4 && hi > mid + 4) {
      out.add(id);
    }
  }
  return out;
}

const COMPACT = "device.sizeClass == 'compact'";
const WIDER = "device.sizeClass != 'compact'";

/** The scaffold whose `bottomNavigationBar` is the navigation bar `barId`. */
function scaffoldWith(n: unknown, barId: string): Obj | undefined {
  if (!isObj(n)) {
    return undefined;
  }
  const slots = isObj(n["slots"]) ? n["slots"] : {};
  if (isObj(slots["bottomNavigationBar"]) && slots["bottomNavigationBar"]["id"] === barId) {
    return n;
  }
  for (const k of kidsOf(n)) {
    const found = scaffoldWith(k, barId);
    if (found !== undefined) {
      return found;
    }
  }
  return undefined;
}

/** Whether the navigation bar `barId` is a page's bottom bar that can be made adaptive, or is adaptive already. */
export function adaptiveState(root: unknown, barId: string): "none" | "ready" | "adaptive" {
  const scaffold = scaffoldWith(root, barId);
  if (scaffold === undefined) {
    return "none";
  }
  const body = (scaffold["slots"] as Obj)["body"];
  const first = isObj(body) && Array.isArray(body["children"]) ? body["children"][0] : undefined;
  return isObj(first) && first["type"] === "NavigationRail" ? "adaptive" : "ready";
}

/** A plain copy of a document value; it reads through the editor's draft proxies, which `structuredClone` cannot. */
const copy = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T;

/** A navigation rail with the bar's destinations, selection and handler, shown from 600 dp, extended from 840 dp. */
function railFor(bar: Obj, id: string): Obj {
  const props = isObj(bar["props"]) ? bar["props"] : {};
  return {
    id,
    type: "NavigationRail",
    props: {
      destinations: copy(props["destinations"] ?? []),
      selectedIndex: copy(props["selectedIndex"] ?? 0),
      labelType: "all",
    },
    responsive: { expanded: { extended: true, labelType: "none" } },
    visible: { $expr: WIDER },
    ...(isObj(bar["events"]) ? { events: copy(bar["events"]) } : {}),
  };
}

/**
 * The recipe that makes the page's navigation bar adaptive: the bar shows on compact screens only, and from 600 dp a
 * navigation rail with the same destinations, selection and handler stands beside the body, extended from 840 dp.
 */
export function adaptiveNavigationRecipe(barId: string, newId: () => string): (root: unknown) => void {
  return (root) => {
    const scaffold = scaffoldWith(root, barId);
    if (scaffold === undefined || adaptiveState(root, barId) !== "ready") {
      return;
    }
    const slots = scaffold["slots"] as Obj;
    const bar = slots["bottomNavigationBar"] as Obj;
    bar["visible"] = { $expr: COMPACT };
    const body = slots["body"];
    slots["body"] = {
      id: newId(),
      type: "Row",
      props: { crossAxisAlignment: "stretch" },
      children: [
        railFor(bar, newId()),
        { id: newId(), type: "VerticalDivider", props: { width: 1 }, visible: { $expr: WIDER } },
        { id: newId(), type: "Expanded", slots: isObj(body) ? { child: body } : {} },
      ],
    };
  };
}

/** The Size classes view's marks with their words, as the overlay draws them. */
export interface SizesOverlay {
  readonly marks: ReadonlyMap<string, SizeMark>;
  /** Each screen's root and the size class tag it carries, e.g. "Medium · 673 dp". */
  readonly screens: readonly { readonly root: string; readonly label: string }[];
  readonly text: (mark: SizeMark) => string;
}
