// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { MockState } from "../preview/switches.ts";
import type { Node } from "./plugin-model.ts";

/**
 * Design-time data for the canvas (DAT-080, PC-23): a source's mock under the state the preview switch selects, and the
 * page's parameters (their mocks, else their defaults) and the page's, plugin's and app's state (their defaults), so a
 * widget bound to `params.name` or `page.count` shows a value, not its placeholder.
 */

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** A data source's mock, by source ID. */
export type SourceMocks = ReadonlyMap<string, unknown>;

/** `data.<source>.<value|loading|error>` then member steps, optionally `?? <JSON literal>`. */
const binding = /^\s*data\.(\w+)\.(value|loading|error)((?:\??\.\w+|\[\d+\])*)(?:\s*\?\?\s*(.+?))?\s*$/;
/** `params`, `page`, `plugin` or `app` then member steps, optionally `?? <JSON literal>`. */
const scoped = /^\s*(params|page|plugin|app)((?:\??\.\w+|\[\d+\])+)(?:\s*\?\?\s*(.+?))?\s*$/;

/** The values the scopes hold at design time: page parameters and the page's, plugin's and app's state. */
export interface DesignScope {
  readonly params: Readonly<Obj>;
  readonly page: Readonly<Obj>;
  readonly plugin: Readonly<Obj>;
  readonly app: Readonly<Obj>;
}

export const emptyScope: DesignScope = { params: {}, page: {}, plugin: {}, app: {} };

const scopeSize = (s: DesignScope) =>
  Object.keys(s.params).length + Object.keys(s.page).length + Object.keys(s.plugin).length + Object.keys(s.app).length;

/** The value at `steps` (`.a?.b[0]`) under `start`; null when a step reaches nothing. */
function walk(start: unknown, steps: string): unknown {
  let current = start;
  for (const s of steps.matchAll(step)) {
    const key = s[1] ?? Number(s[2]);
    current = Array.isArray(current) && typeof key === "number" ? current[key] : isObj(current) ? current[key] : null;
  }
  return current;
}
const step = /\??\.(\w+)|\[(\d+)\]/g;

/** What a source holds when it is `empty`: no items, no fields, no text. */
function emptyOf(mock: unknown): unknown {
  return Array.isArray(mock) ? [] : isObj(mock) ? {} : typeof mock === "string" ? "" : null;
}

/** The `.value`, `.loading` and `.error` of a source under a mock state. */
function stateOf(mock: unknown, state: MockState): Readonly<Record<"value" | "loading" | "error", unknown>> {
  switch (state) {
    case "loading":
      return { value: null, loading: true, error: null };
    case "empty":
      return { value: emptyOf(mock), loading: false, error: null };
    case "error":
      return { value: null, loading: false, error: true };
    case "success":
      return { value: mock, loading: false, error: null };
  }
}

function literal(text: string | undefined): unknown {
  if (text === undefined) {
    return undefined;
  }
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

/**
 * The value of one binding expression under the selected mock states, `undefined` when the expression is not a
 * plain data-source path (it is left as it is) and `null` when the path reaches nothing.
 */
export function evaluateMockBinding(
  expr: string,
  mocks: SourceMocks,
  states: Readonly<Record<string, MockState>>,
  scope: DesignScope = emptyScope,
): unknown {
  const inScope = scoped.exec(expr);
  if (inScope !== null) {
    const root = scope[inScope[1] as keyof DesignScope];
    const first = /^\??\.(\w+)/.exec(inScope[2] ?? "")?.[1];
    if (first === undefined || !(first in root)) {
      return undefined;
    }
    return walk(root, inScope[2] ?? "") ?? literal(inScope[3]) ?? null;
  }
  const match = binding.exec(expr);
  const id = match?.[1];
  if (match === null || id === undefined || !mocks.has(id)) {
    return undefined;
  }
  const current = walk(stateOf(mocks.get(id), states[id] ?? "success")[match[2] as "value"], match[3] ?? "");
  return current ?? literal(match[4]) ?? null;
}

/** The mock value of a `{"$expr": "…"}` prop, `undefined` when `value` is not a binding this module can read. */
interface Env {
  readonly mocks: SourceMocks;
  readonly states: Readonly<Record<string, MockState>>;
  readonly scope: DesignScope;
}

function resolved(value: unknown, env: Env): unknown {
  const expr = isObj(value) && Object.keys(value).length === 1 ? value["$expr"] : undefined;
  return typeof expr === "string" ? evaluateMockBinding(expr, env.mocks, env.states, env.scope) : undefined;
}

function bindValue(value: unknown, env: Env): unknown {
  if (Array.isArray(value)) {
    return value.map((v) => {
      const bound = resolved(v, env);
      return bound === undefined ? bindValue(v, env) : bound;
    });
  }
  if (!isObj(value)) {
    return value;
  }
  const out: Obj = {};
  for (const [key, child] of Object.entries(value)) {
    const bound = resolved(child, env);
    if (bound === undefined) {
      out[key] = bindValue(child, env);
    } else if (bound !== null) {
      out[key] = bound;
    }
  }
  return out;
}

/**
 * The widget tree with each `{"$expr": "data.<source>.value…"}` prop replaced by the source's mock under the state
 * selected for it (DAT-080), so bound widgets show the mock values. A binding that reaches nothing (a loading or
 * failed source) drops the prop; expressions that are not plain data paths stay as they are.
 */
export function bindMocks(
  root: Node | undefined,
  mocks: SourceMocks,
  states: Readonly<Record<string, MockState>>,
  scope: DesignScope = emptyScope,
): Node | undefined {
  return root === undefined || (mocks.size === 0 && scopeSize(scope) === 0)
    ? root
    : (bindValue(root, { mocks, states, scope }) as Node);
}

/** A prop bound to an expression: its path in the node's props (`style.color`) and the expression. */
export interface BoundProp {
  readonly prop: string;
  readonly expr: string;
}

function collectBound(value: unknown, path: string, out: BoundProp[]): void {
  if (isObj(value) && Object.keys(value).length === 1 && typeof value["$expr"] === "string") {
    out.push({ prop: path, expr: value["$expr"] });
    return;
  }
  if (isObj(value)) {
    for (const [k, v] of Object.entries(value)) {
      collectBound(v, path === "" ? k : `${path}.${k}`, out);
    }
  }
}

/** The children and slot contents of a node, in document order. */
function kidsOf(n: Obj): unknown[] {
  const kids = Array.isArray(n["children"]) ? n["children"] : [];
  const slots = isObj(n["slots"]) ? Object.values(n["slots"]) : [];
  return [...kids, ...slots.flatMap((v) => (Array.isArray(v) ? v : [v]))];
}

/** A node's props bound to expressions, and its `visible` when it is one. */
function boundOf(n: Obj): BoundProp[] {
  const list: BoundProp[] = [];
  collectBound(n["props"], "", list);
  if (typeof n["visible"] === "string") {
    list.push({ prop: "visible", expr: n["visible"] });
  }
  return list;
}

/** The bound props of every node under `root` (PC-23): node id to its props bound to expressions, and `visible`. */
export function boundPropsOf(root: Node | undefined): Map<string, readonly BoundProp[]> {
  const out = new Map<string, readonly BoundProp[]>();
  const visit = (n: unknown) => {
    if (!isObj(n) || typeof n["id"] !== "string") {
      return;
    }
    const list = boundOf(n);
    if (list.length > 0) {
      out.set(n["id"], list);
    }
    kidsOf(n).forEach(visit);
  };
  visit(root);
  return out;
}
