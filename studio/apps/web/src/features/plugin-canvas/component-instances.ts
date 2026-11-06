// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Component instances on the canvas (PC-37, STU-084, STU-085, SCH-030): an instance — a node that names a component
 * and the version it was built against — is drawn as the component's tree. Its `props.<name>` bindings take the
 * instance's values, else the component's defaults; each `Slot` the instance fills shows the instance's own widgets,
 * else the slot's fallback; and the drawn copies of the component's widgets carry canvas-only ids under the instance
 * (`<instance>~c<id>`), so choosing any part of it selects the instance. What the canvas says of each instance: its
 * component, the version it uses and the newest, and how many props and slots it overrides.
 */

import { COPY_MARK } from "./list-preview.ts";
import { type DocLike, jsonOf } from "./look.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** How deep instances may nest in one another before the canvas stops expanding (a cycle draws as a placeholder). */
const MAX_DEPTH = 8;

/** The app's components by id. */
export function componentsOf(documents: readonly DocLike[]): ReadonlyMap<string, Obj> {
  return new Map(jsonOf(documents, "component").flatMap((c) => (typeof c["id"] === "string" ? [[c["id"], c]] : [])));
}

/** The instance a node is, when it is one: the component and version it names. */
export function instanceRef(n: unknown): { readonly id: string; readonly version: number } | undefined {
  if (!isObj(n) || n["type"] !== undefined || !isObj(n["component"])) {
    return undefined;
  }
  const c = n["component"];
  return typeof c["id"] === "string"
    ? { id: c["id"], version: typeof c["version"] === "number" ? c["version"] : 1 }
    : undefined;
}

/** The fields of an instance that apply to it wherever it stands, kept on its drawn root. */
const OWN = ["visible", "responsive", "localeOverrides", "semantics", "animation", "onlyIn", "x-studio"] as const;

interface Expansion {
  readonly components: ReadonlyMap<string, Obj>;
  readonly depth: number;
}

function expandList(list: readonly unknown[], e: Expansion): readonly unknown[] {
  const next = list.map((x) => expandNode(x, e));
  return next.every((x, i) => x === list[i]) ? list : next;
}

/** A node with its instances expanded; the node itself when nothing under it is an instance. */
function expandNode(n: unknown, e: Expansion): unknown {
  if (!isObj(n)) {
    return n;
  }
  if (instanceRef(n) !== undefined) {
    return instanceTree(n, e);
  }
  let out: Obj | undefined;
  if (Array.isArray(n["children"])) {
    const kids = expandList(n["children"], e);
    if (kids !== n["children"]) {
      out = { ...(out ?? n), children: kids };
    }
  }
  if (isObj(n["slots"])) {
    const slots = n["slots"];
    const next = Object.fromEntries(
      Object.entries(slots).map(([k, v]) => [k, Array.isArray(v) ? expandList(v, e) : expandNode(v, e)]),
    );
    if (Object.keys(slots).some((k) => next[k] !== slots[k])) {
      out = { ...(out ?? n), slots: next };
    }
  }
  return out ?? n;
}

/** A `props.<name>` binding's value from the instance (or the default); undefined when `v` is not one or has none. */
function boundValue(v: Obj, values: Obj): unknown {
  const bound = typeof v["$expr"] === "string" ? /^\s*props\.(\w+)\s*$/.exec(v["$expr"]) : null;
  return bound === null ? undefined : values[bound[1] as string];
}

/** What a filled `Slot` shows: the instance's own widgets, keeping their ids; undefined for a slot it does not fill. */
function slotFill(v: Obj, inst: Obj, e: Expansion): unknown {
  if (v["type"] !== "Slot" || !isObj(v["props"]) || typeof v["props"]["name"] !== "string") {
    return undefined;
  }
  const fill = isObj(inst["slots"]) ? inst["slots"][v["props"]["name"]] : undefined;
  if (fill === undefined) {
    return undefined;
  }
  return Array.isArray(fill)
    ? { id: `${String(inst["id"])}${COPY_MARK}s${String(v["id"])}`, type: "Column", children: expandList(fill, e) }
    : expandNode(fill, e);
}

/** The component's own widget copied under the instance: its id marked, its prop bindings and slots filled. */
function copyPart(v: unknown, inst: Obj, values: Obj, e: Expansion): unknown {
  if (Array.isArray(v)) {
    return v.map((x) => copyPart(x, inst, values, e));
  }
  if (!isObj(v)) {
    return v;
  }
  const bound = boundValue(v, values);
  if (bound !== undefined) {
    return bound;
  }
  const filled = slotFill(v, inst, e);
  if (filled !== undefined) {
    return filled;
  }
  const out: Obj = {};
  for (const [k, x] of Object.entries(v)) {
    out[k] =
      k === "id" && typeof x === "string" ? `${String(inst["id"])}${COPY_MARK}c${x}` : copyPart(x, inst, values, e);
  }
  return instanceRef(out) === undefined ? out : instanceTree(out, { ...e, depth: e.depth + 1 });
}

/** An instance drawn as its component's tree, its root keeping the instance's id; as it is when the component is unknown. */
function instanceTree(inst: Obj, e: Expansion): Obj {
  const ref = instanceRef(inst);
  const comp = ref === undefined ? undefined : e.components.get(ref.id);
  if (comp === undefined || !isObj(comp["root"]) || e.depth >= MAX_DEPTH) {
    return inst;
  }
  const declared = Array.isArray(comp["props"]) ? comp["props"].filter(isObj) : [];
  const values: Obj = {
    ...Object.fromEntries(
      declared.flatMap((p) => (p["default"] === undefined ? [] : [[String(p["name"]), p["default"]]])),
    ),
    ...(isObj(inst["props"]) ? inst["props"] : {}),
  };
  const tree = copyPart(comp["root"], inst, values, { ...e, depth: e.depth + 1 }) as Obj;
  const own = Object.fromEntries(OWN.flatMap((k) => (inst[k] === undefined ? [] : [[k, inst[k]]])));
  return { ...tree, ...own, id: inst["id"] };
}

/** The tree with every component instance drawn as its component (PC-37); the tree itself when it has none. */
export function expandInstances<T>(root: T, components: ReadonlyMap<string, Obj>): T {
  return components.size === 0 ? root : (expandNode(root, { components, depth: 0 }) as T);
}

/** What the canvas says of an instance. */
export interface InstanceMark {
  readonly component: string;
  readonly name: string;
  readonly version: number;
  /** The component's newest version the app has; above `version`, the app has not adopted it here yet. */
  readonly latest: number;
  /** The props and slots the instance sets itself. */
  readonly overrides: number;
  /** Whether the app has the component at all. */
  readonly known: boolean;
}

const kidsOf = (n: Obj): unknown[] => [
  ...(isObj(n["slots"]) ? Object.values(n["slots"]).flatMap((v) => (Array.isArray(v) ? v : [v])) : []),
  ...(Array.isArray(n["children"]) ? n["children"] : []),
];

/** What the canvas says of one instance. */
function markOf(
  n: Obj,
  ref: { readonly id: string; readonly version: number },
  components: ReadonlyMap<string, Obj>,
): InstanceMark {
  const comp = components.get(ref.id);
  const count = (v: unknown) => Object.keys(isObj(v) ? v : {}).length;
  return {
    component: ref.id,
    name: typeof comp?.["name"] === "string" ? comp["name"] : ref.id,
    version: ref.version,
    latest: typeof comp?.["version"] === "number" ? comp["version"] : ref.version,
    overrides: count(n["props"]) + count(n["slots"]),
    known: comp !== undefined,
  };
}

/** Every instance under `roots`, by id, with its component's name and versions and its overrides. */
export function instanceMarks(
  roots: readonly unknown[],
  components: ReadonlyMap<string, Obj>,
): Map<string, InstanceMark> {
  const out = new Map<string, InstanceMark>();
  const visit = (n: unknown) => {
    if (!isObj(n)) {
      return;
    }
    const ref = instanceRef(n);
    if (ref !== undefined && typeof n["id"] === "string") {
      out.set(n["id"], markOf(n, ref, components));
    }
    kidsOf(n).forEach(visit);
  };
  roots.forEach(visit);
  return out;
}
