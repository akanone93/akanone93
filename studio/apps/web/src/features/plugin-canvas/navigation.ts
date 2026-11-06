// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Navigation on the canvas (PC-28, NAV-001, NAV-010): what a link passes to the page it opens — each parameter the
 * page declares, typed, required ones marked missing until set — edited from the link's lane; the transition the
 * page opens with; and the screens arranged by flow, a column per step from the entry page.
 */

import type { NavigateLink } from "./links.ts";
import { findNode } from "./select/edits.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** A parameter a page declares (SCH-022, SCH-024). */
export interface DeclaredParam {
  readonly name: string;
  readonly type: string;
  readonly required: boolean;
  readonly default?: unknown;
}

/** The parameters `page` declares. */
export function declaredParams(page: unknown): DeclaredParam[] {
  const params = isObj(page) && Array.isArray(page["params"]) ? page["params"] : [];
  return params.flatMap((p) =>
    isObj(p) && typeof p["name"] === "string"
      ? [
          {
            name: p["name"],
            type: String(p["type"] ?? "string"),
            required: p["required"] === true,
            ...(p["default"] === undefined ? {} : { default: p["default"] }),
          },
        ]
      : [],
  );
}

/** The step a link stands for, in node `id`'s handlers. */
function stepOf(root: unknown, id: string, link: NavigateLink): Obj | undefined {
  const node = findNode(root, id)?.node as Obj | undefined;
  const events = isObj(node?.["events"]) ? (node["events"] as Obj) : {};
  const handler = events[link.event];
  const steps = isObj(handler) && Array.isArray(handler["steps"]) ? handler["steps"] : [];
  const step = steps[link.step];
  return isObj(step) ? step : undefined;
}

/** The parameters a link passes. */
export function linkParams(root: unknown, id: string, link: NavigateLink): Obj {
  const input = stepOf(root, id, link)?.["input"];
  return isObj(input) && isObj(input["params"]) ? input["params"] : {};
}

/** The declared parameters a link leaves out though the page requires them (and gives no default). */
export function missingParams(declared: readonly DeclaredParam[], passed: Readonly<Obj>): string[] {
  return declared
    .filter((d) => d.required && d.default === undefined && passed[d.name] === undefined)
    .map((d) => d.name);
}

/**
 * A parameter's value as typed in the lane (PC-28): a number for an int or double, true or false for a bool, a quoted
 * text as text, anything else an expression (`item.id`, `page.tab + 1`); empty clears it.
 */
export function paramValue(type: string, text: string): unknown {
  const t = text.trim();
  if (t === "") {
    return undefined;
  }
  const base = type.replace(/\?$/, "");
  if ((base === "int" || base === "double") && /^-?\d+(\.\d+)?$/.test(t)) {
    return base === "int" ? Math.trunc(Number(t)) : Number(t);
  }
  if (base === "bool" && (t === "true" || t === "false")) {
    return t === "true";
  }
  const quoted = /^"(.*)"$/.exec(t);
  if (quoted !== null) {
    return quoted[1];
  }
  return { $expr: t };
}

/** A parameter's value as the lane shows it for editing: the expression, a text quoted, a literal as JSON. */
export function paramText(v: unknown): string {
  if (v === undefined) {
    return "";
  }
  if (isObj(v) && typeof v["$expr"] === "string") {
    return v["$expr"];
  }
  return typeof v === "string" ? `"${v}"` : JSON.stringify(v);
}

/** The recipe that sets (or, with undefined, removes) parameter `name` of the link's step. */
export function setLinkParamRecipe(
  id: string,
  link: NavigateLink,
  name: string,
  value: unknown,
): (root: unknown) => void {
  return (root) => {
    const step = stepOf(root, id, link);
    if (step === undefined) {
      return;
    }
    const input: Obj = isObj(step["input"]) ? step["input"] : {};
    const params: Obj = isObj(input["params"]) ? input["params"] : {};
    if (value === undefined) {
      delete params[name];
    } else {
      params[name] = value;
    }
    if (Object.keys(params).length === 0) {
      delete input["params"];
    } else {
      input["params"] = params;
    }
    step["input"] = input;
  };
}

/** The transition a page opens with; `platform` when it declares none. */
export function transitionOf(page: unknown): string {
  const options = isObj(page) && isObj(page["routeOptions"]) ? page["routeOptions"] : {};
  return typeof options["transition"] === "string" ? options["transition"] : "platform";
}

/** The recipe that sets a page's transition; `platform` removes it. */
export function setTransitionRecipe(transition: string): (page: Obj) => void {
  return (page) => {
    const options: Obj = isObj(page["routeOptions"]) ? page["routeOptions"] : {};
    if (transition === "platform") {
      delete options["transition"];
    } else {
      options["transition"] = transition;
    }
    if (Object.keys(options).length === 0) {
      delete page["routeOptions"];
    } else {
      page["routeOptions"] = options;
    }
  };
}

/** Each screen's step from the entry along the links (breadth first); screens it does not reach come last. */
export function flowDepths(
  ids: readonly string[],
  entry: string | undefined,
  edges: ReadonlyMap<string, readonly string[]>,
): Map<string, number> {
  const known = new Set(ids);
  const depth = new Map<string, number>();
  const queue: string[] = entry !== undefined && known.has(entry) ? [entry] : [];
  if (queue[0] !== undefined) {
    depth.set(queue[0], 0);
  }
  while (queue.length > 0) {
    const id = queue.shift() as string;
    for (const next of edges.get(id) ?? []) {
      if (known.has(next) && !depth.has(next)) {
        depth.set(next, (depth.get(id) ?? 0) + 1);
        queue.push(next);
      }
    }
  }
  const last = Math.max(-1, ...depth.values()) + 1;
  for (const id of ids) {
    if (!depth.has(id)) {
      depth.set(id, last);
    }
  }
  return depth;
}

/**
 * The screens arranged by flow (PC-28): a column per step from the entry, left to right, each column's screens stacked
 * in their listed order; `size` gives each screen's width and height, `gap` the space between.
 */
export function flowLayout(
  ids: readonly string[],
  entry: string | undefined,
  edges: ReadonlyMap<string, readonly string[]>,
  size: (id: string) => { readonly w: number; readonly h: number },
  gap: number,
): Map<string, { x: number; y: number }> {
  const depth = flowDepths(ids, entry, edges);
  const columns: string[][] = [];
  for (const id of ids) {
    const d = depth.get(id) ?? 0;
    const column = columns[d] ?? [];
    column.push(id);
    columns[d] = column;
  }
  const out = new Map<string, { x: number; y: number }>();
  let x = 0;
  for (const column of columns.filter((c) => c !== undefined)) {
    let y = 0;
    let wide = 0;
    for (const id of column) {
      out.set(id, { x, y });
      const s = size(id);
      y += s.h + gap;
      wide = Math.max(wide, s.w);
    }
    x += wide + gap;
  }
  return out;
}
