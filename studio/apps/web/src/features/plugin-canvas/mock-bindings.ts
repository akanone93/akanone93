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
/** `item` (then member steps) or `index`, inside an item template, optionally `?? <JSON literal>`. */
const itemPath = /^\s*(item|index)((?:\??\.\w+|\[\d+\])*)(?:\s*\?\?\s*(.+?))?\s*$/;
/** A scope root (`params`, `page`, `props`, …) then member steps, optionally `?? <JSON literal>`. */
const scoped =
  /^\s*(params|page|plugin|app|device|flags|props|component)((?:\??\.\w+|\[\d+\])+)(?:\s*\?\?\s*(.+?))?\s*$/;

/** `item` or `index` as a root, not a member (`page.item`): an expression that reads a list item. */
const ITEM_ROOT = /(?<![.\w$])(item|index)\b/;

/** Whether an expression reads a list item (`item`, `index`), so only a copy of its item template has its value. */
export function readsItem(expr: string): boolean {
  return ITEM_ROOT.test(expr);
}

/** The values the scopes hold at design time: page parameters and the page's, plugin's and app's state. */
export interface DesignScope {
  readonly params: Readonly<Obj>;
  readonly page: Readonly<Obj>;
  readonly plugin: Readonly<Obj>;
  readonly app: Readonly<Obj>;
  /** PXL's `device` root at design time (PC-33): the frame's size class and the preview's switches. */
  readonly device?: Readonly<Obj>;
  /** PXL's `flags` root at design time: each flag the app declares at its default (build log row 138). */
  readonly flags?: Readonly<Obj>;
  /** Inside a component instance's drawn tree: the props it was given, at their design-time values (ADR-0112). */
  readonly props?: Readonly<Obj>;
  /** Inside a component instance's drawn tree: the component's state, at its defaults. */
  readonly component?: Readonly<Obj>;
  /** The scope of a component's own expressions, by component id; absent, they stay as they are. */
  readonly componentScope?: (id: string) => ComponentScope | undefined;
  /** Inside a list's item template (PC-24): the item a copy is built for, and its index. */
  readonly item?: unknown;
  readonly index?: number;
  /**
   * The PXL engine's design-time value of an expression the readers here do not resolve (ADR-0112): a value, or
   * undefined until the engine has answered, or when the expression does not evaluate. Absent, such expressions stay.
   */
  readonly evaluate?: (expr: string, scope: DesignScope) => unknown;
}

/**
 * The field of a component instance's drawn root that names its component and the props it was given (the instance's
 * values over the defaults, as written: literals and page expressions), so its component's own expressions are
 * evaluated with them (ADR-0112): a list, outermost first, where a component's root is itself an instance. Canvas-only:
 * the binder reads it and leaves it out.
 */
export const INSTANCE_MARK = "x-instance";

/** The field of a slot's fill: the instance's own widgets, bound in the scope the instance stands in. Canvas-only. */
export const OUTER_MARK = "x-outer";

/** What an instance's drawn root says of it: its component and the props it was given. */
export interface InstanceOf {
  readonly component: string;
  readonly props: Readonly<Obj>;
}

/** A component's design-time scope (ADR-0112): its state's defaults and the evaluator of its own expressions. */
export interface ComponentScope {
  readonly state: Readonly<Obj>;
  readonly evaluate?: (expr: string, scope: DesignScope) => unknown;
}

/** The roots the readers here resolve a path under. */
type ScopeRoot = "params" | "page" | "plugin" | "app" | "device" | "flags" | "props" | "component";

export const emptyScope: DesignScope = { params: {}, page: {}, plugin: {}, app: {} };

const scopeSize = (s: DesignScope) =>
  Object.keys(s.params).length +
  Object.keys(s.page).length +
  Object.keys(s.plugin).length +
  Object.keys(s.app).length +
  Object.keys(s.flags ?? {}).length +
  Object.keys(s.props ?? {}).length;

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

/** An `item…` or `index` path inside an item template; undefined when `expr` is not one. */
function itemValue(expr: string, scope: DesignScope): unknown {
  const m = scope.index === undefined ? null : itemPath.exec(expr);
  if (m === null) {
    return undefined;
  }
  const value = m[1] === "index" ? (m[2] === "" ? scope.index : null) : walk(scope.item, m[2] ?? "");
  return value ?? literal(m[3]) ?? null;
}

/** A `params`, `page`, `plugin`, `app` or `device` path; undefined when `expr` is not one or names nothing there. */
function scopeValue(expr: string, scope: DesignScope): unknown {
  const m = scoped.exec(expr);
  if (m === null) {
    return undefined;
  }
  const root = scope[m[1] as ScopeRoot];
  const first = /^\??\.(\w+)/.exec(m[2] ?? "")?.[1];
  return root === undefined || first === undefined || !(first in root)
    ? undefined
    : (walk(root, m[2] ?? "") ?? literal(m[3]) ?? null);
}

/** A `data.<source>` path under the source's mock state; undefined when `expr` is not one of a known source. */
function dataValue(expr: string, mocks: SourceMocks, states: Readonly<Record<string, MockState>>): unknown {
  const match = binding.exec(expr);
  const id = match?.[1];
  if (match === null || id === undefined || !mocks.has(id)) {
    return undefined;
  }
  const current = walk(stateOf(mocks.get(id), states[id] ?? "success")[match[2] as "value"], match[3] ?? "");
  return current ?? literal(match[4]) ?? null;
}

/**
 * The value of an expression that is a constant list or map literal written as JSON (`[{"name": "Nordvia"}]`), as a
 * list's `items` holds before a data source feeds it (build log #83); undefined for anything else. Parsing runs no
 * code, and only a list or a map is read, so a path such as `state.count` never reaches it.
 */
function constantLiteral(expr: string): unknown {
  const text = expr.trim();
  if (!(text.startsWith("[") || text.startsWith("{"))) {
    return undefined;
  }
  try {
    const value: unknown = JSON.parse(text);
    return typeof value === "object" && value !== null ? value : undefined;
  } catch {
    return undefined;
  }
}

/**
 * The value of one binding expression under the selected mock states, `undefined` when the expression is not a
 * plain path or a constant literal this module reads and the scope's PXL engine has no value for it yet (it is left as
 * it is), and `null` when the path reaches nothing.
 */
export function evaluateMockBinding(
  expr: string,
  mocks: SourceMocks,
  states: Readonly<Record<string, MockState>>,
  scope: DesignScope = emptyScope,
  /** Whether the scope's PXL engine is asked when the readers here do not resolve `expr`; not for a part of one. */
  engine = true,
): unknown {
  // `null` (a path reaching nothing) is an answer; only `undefined` passes the expression to the next reader.
  const constant = constantLiteral(expr);
  if (constant !== undefined) {
    return constant;
  }
  const item = itemValue(expr, scope);
  if (item !== undefined) {
    return item;
  }
  const scoped = scopeValue(expr, scope);
  if (scoped !== undefined) {
    return scoped;
  }
  const data = dataValue(expr, mocks, states);
  return data !== undefined || !engine ? data : scope.evaluate?.(expr, scope);
}

/** The mock value of a `{"$expr": "…"}` prop, `undefined` when `value` is not a binding this module can read. */
interface Env {
  readonly mocks: SourceMocks;
  readonly states: Readonly<Record<string, MockState>>;
  readonly scope: DesignScope;
  /** Inside a component instance's tree: the environment the instance stands in. */
  readonly outer?: Env;
}

/**
 * A binding's value in `env`, else in the environments around it: a component's tree also holds what its instance
 * wrote (a prop's expression in place of a plain `props.<name>`), which only the instance's scope evaluates.
 */
function resolved(value: unknown, env: Env): unknown {
  const expr = isObj(value) && Object.keys(value).length === 1 ? value["$expr"] : undefined;
  if (typeof expr !== "string") {
    return undefined;
  }
  for (let at: Env | undefined = env; at !== undefined; at = at.outer) {
    const found = evaluateMockBinding(expr, at.mocks, at.states, at.scope);
    if (found !== undefined) {
      return found;
    }
  }
  return undefined;
}

/**
 * The environment of a component instance's drawn tree (ADR-0112): its props at their values in the instance's own
 * scope, its component's state and the evaluator of its own expressions; a list item's values stay outside.
 */
function instanceEnv(mark: unknown, env: Env): Env {
  const inst = isObj(mark) ? mark : {};
  const given = isObj(inst["props"]) ? (bindValue(inst["props"], env) as Obj) : {};
  const component = typeof inst["component"] === "string" ? env.scope.componentScope?.(inst["component"]) : undefined;
  const { item: _item, index: _index, evaluate: _evaluate, ...outer } = env.scope;
  const scope: DesignScope = {
    ...outer,
    props: given,
    component: component?.state ?? {},
    ...(component?.evaluate === undefined ? {} : { evaluate: component.evaluate }),
  };
  return { ...env, scope, outer: env };
}

function bindValue(value: unknown, env: Env): unknown {
  if (isObj(value) && value[INSTANCE_MARK] !== undefined) {
    const { [INSTANCE_MARK]: marks, ...node } = value;
    const inner = (Array.isArray(marks) ? marks : [marks]).reduce<Env>((at, mark) => instanceEnv(mark, at), env);
    return bindValue(node, inner);
  }
  if (isObj(value) && value[OUTER_MARK] !== undefined) {
    const { [OUTER_MARK]: _outer, ...node } = value;
    return bindValue(node, env.outer ?? env);
  }
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
  return root === undefined ||
    (mocks.size === 0 && scopeSize(scope) === 0 && scope.evaluate === undefined && scope.componentScope === undefined)
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
  const visible = n["visible"];
  if (isObj(visible) && typeof visible["$expr"] === "string") {
    list.push({ prop: "visible", expr: visible["$expr"] });
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
