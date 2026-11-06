// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Conditions on the canvas (PC-25, SCH-023): an `If` draws its `then` or `else` branch, a `Match` the branch of its
 * value, a `ForEach` one copy of its template per item, and a widget whose `visible` is false is left out — each as the
 * device would at design time, from page parameters' mocks, state defaults and data mocks. A condition the canvas
 * cannot work out draws the first branch and keeps the widget. The inspector can pick a branch to draw instead.
 */

import { SIZE_CLASSES, type SizeClass } from "@plux/layout";
import type { MockState } from "../preview/switches.ts";
import { COPY_MARK, MAX_DRAWN_ITEMS, templateCopy } from "./list-preview.ts";
import { type DesignScope, emptyScope, evaluateMockBinding, type SourceMocks } from "./mock-bindings.ts";
import type { Node } from "./plugin-model.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** A branch picked in the inspector: `then`, `else`, `case:<i>`, `otherwise`, `shown` or `hidden`. */
export type BranchPick = string;

interface Env {
  readonly mocks: SourceMocks;
  readonly states: Readonly<Record<string, MockState>>;
  readonly scope: DesignScope;
  readonly picks: ReadonlyMap<string, BranchPick>;
}

const exprOf = (v: unknown): string | undefined =>
  typeof v === "string" ? v : isObj(v) && typeof v["$expr"] === "string" ? v["$expr"] : undefined;

/** A literal operand: JSON, or a PXL string in single quotes (`'compact'`); undefined when it is not one. */
function literalOf(text: string): unknown {
  const t = text.trim();
  const single = /^'((?:[^'\\]|\\.)*)'$/.exec(t);
  if (single !== null) {
    return (single[1] ?? "").replace(/\\(.)/g, "$1");
  }
  try {
    return JSON.parse(t);
  } catch {
    return undefined;
  }
}

/** The design-time value of an operand: a JSON literal, else a path the scope reads; undefined when unknown. */
function operand(text: string, env: Env): unknown {
  const literal = literalOf(text);
  if (literal !== undefined) {
    return literal;
  }
  const v = evaluateMockBinding(text.trim(), env.mocks, env.states, env.scope);
  return v === null ? undefined : v;
}

const COMPARE = /^(.+?)\s*(==|!=|>=|<=|>|<)\s*(.+)$/;

function compare(a: unknown, op: string, b: unknown): boolean | undefined {
  if (a === undefined || b === undefined) {
    return undefined;
  }
  if (op === "==" || op === "!=") {
    return (JSON.stringify(a) === JSON.stringify(b)) === (op === "==");
  }
  if (typeof a !== "number" || typeof b !== "number") {
    return undefined;
  }
  return op === ">" ? a > b : op === "<" ? a < b : op === ">=" ? a >= b : a <= b;
}

const truthy = (v: unknown): boolean | undefined =>
  v === undefined
    ? undefined
    : !(v === false || v === null || v === 0 || v === "" || (Array.isArray(v) && v.length === 0));

/** `path.isEmpty` or `path.isNotEmpty`: whether the list or text is empty; undefined when `t` is not one. */
function emptiness(t: string, env: Env): boolean | undefined | null {
  const m = /^(.+)\.(isEmpty|isNotEmpty)$/.exec(t);
  if (m === null) {
    return null;
  }
  const v = operand(m[1] ?? "", env);
  const n = Array.isArray(v) || typeof v === "string" ? v.length : undefined;
  return n === undefined ? undefined : (n === 0) === (m[2] === "isEmpty");
}

/** One term: `!term`, a comparison, `path.isEmpty`/`isNotEmpty`, or a path or literal read as true or false. */
function term(text: string, env: Env): boolean | undefined {
  const t = text.trim();
  if (t.startsWith("!")) {
    const inner = term(t.slice(1), env);
    return inner === undefined ? undefined : !inner;
  }
  const empty = emptiness(t, env);
  if (empty !== null) {
    return empty;
  }
  const c = COMPARE.exec(t);
  return c === null ? truthy(operand(t, env)) : compare(operand(c[1] ?? "", env), c[2] ?? "", operand(c[3] ?? "", env));
}

/**
 * A condition's design-time value: terms joined by `&&` or `||` (no parentheses), each a comparison, a negation or a
 * value read as true or false; undefined when any term is unknown.
 */
export function evaluateCondition(
  expr: string,
  mocks: SourceMocks,
  states: Readonly<Record<string, MockState>>,
  scope: DesignScope = emptyScope,
): boolean | undefined {
  const env: Env = { mocks, states, scope, picks: new Map() };
  if (/[()]/.test(expr)) {
    return undefined;
  }
  const any = expr.split("||").map((part) => part.split("&&").map((t) => term(t, env)));
  if (any.some((all) => all.every((v) => v === true))) {
    return true;
  }
  return any.every((all) => all.some((v) => v === false)) ? false : undefined;
}

const conditionOf = (value: unknown, env: Env): boolean | undefined =>
  typeof value === "boolean"
    ? value
    : exprOf(value) === undefined
      ? undefined
      : evaluateCondition(exprOf(value) as string, env.mocks, env.states, env.scope);

const READS_SIZE_CLASS = /\bdevice\??\.sizeClass\b/;

/**
 * The size classes a condition holds in, when it reads `device.sizeClass` and is not the same in all three (PC-33):
 * the layout, which knows each frame's width, then draws what that frame's size class shows. Undefined otherwise.
 */
function sizeClassesOf(value: unknown, env: Env): SizeClass[] | undefined {
  const expr = exprOf(value);
  if (expr === undefined || !READS_SIZE_CLASS.test(expr)) {
    return undefined;
  }
  const at = SIZE_CLASSES.map((sc) =>
    conditionOf(value, { ...env, scope: { ...env.scope, device: { ...env.scope.device, sizeClass: sc } } }),
  );
  return at.every((v) => v === at[0]) ? undefined : SIZE_CLASSES.filter((_, i) => at[i] !== false);
}

/** The size classes a widget is shown in, when its `visible` depends on the size class and no pick decides it. */
function onlyInOf(node: Obj, env: Env): SizeClass[] | undefined {
  const pick = env.picks.get(String(node["id"]));
  return pick === "shown" || pick === "hidden" || node["visible"] === undefined
    ? undefined
    : sizeClassesOf(node["visible"], env);
}

/** Whether a widget is drawn: its pick, else its `visible` when the canvas can work it out, else shown. */
function shown(node: Obj, env: Env): boolean {
  const pick = env.picks.get(String(node["id"]));
  if (pick === "shown" || pick === "hidden") {
    return pick === "shown";
  }
  const only = onlyInOf(node, env);
  if (only !== undefined) {
    return only.length > 0;
  }
  return node["visible"] === undefined || conditionOf(node["visible"], env) !== false;
}

/** An `If` on the size class as a `Responsive`: each size class draws the branch its condition picks there. */
function responsiveIf(node: Obj, thenIn: readonly SizeClass[]): Obj {
  const slots = isObj(node["slots"]) ? node["slots"] : {};
  const variants: Obj = {};
  for (const sc of SIZE_CLASSES) {
    const branch = thenIn.includes(sc) ? slots["then"] : slots["else"];
    if (branch !== undefined) {
      variants[sc] = branch;
    }
  }
  return { id: node["id"], type: "Responsive", slots: variants };
}

/** An `If` as drawn: its condition the branch picked, or worked out (true when unknown). */
function drawnIf(node: Obj, env: Env): Obj {
  const pick = env.picks.get(String(node["id"]));
  const props = isObj(node["props"]) ? node["props"] : {};
  const thenIn = pick === undefined ? sizeClassesOf(props["condition"], env) : undefined;
  if (thenIn !== undefined) {
    return responsiveIf(node, thenIn);
  }
  const value = pick === "then" || pick === "else" ? pick === "then" : (conditionOf(props["condition"], env) ?? true);
  return { ...node, props: { ...props, condition: value } };
}

/** A `Match` as drawn: its value the case picked, or worked out; `otherwise` when no case names it. */
function drawnMatch(node: Obj, env: Env): Obj {
  const pick = env.picks.get(String(node["id"]));
  const props = isObj(node["props"]) ? node["props"] : {};
  const cases = Array.isArray(props["cases"]) ? (props["cases"] as unknown[]) : [];
  let value: unknown = props["value"];
  if (pick?.startsWith("case:") === true) {
    value = cases[Number(pick.slice(5))];
  } else if (pick === "otherwise") {
    value = `${COPY_MARK}otherwise`;
  } else if (exprOf(value) !== undefined && typeof value !== "string") {
    value = evaluateMockBinding(exprOf(value) as string, env.mocks, env.states, env.scope) ?? value;
  }
  return { ...node, props: { ...props, value } };
}

/** The axis a parent lays its children along, for a `ForEach` drawn as a flex of its copies. */
function axisOf(parent: Obj | undefined): "horizontal" | "vertical" {
  const type = parent?.["type"];
  const direction = isObj(parent?.["props"]) ? parent["props"]["direction"] : undefined;
  if (type === "Row") {
    return "horizontal";
  }
  return (type === "Wrap" || type === "Flex") && direction !== "vertical" ? "horizontal" : "vertical";
}

/** A `ForEach` as drawn: a flex along its parent's axis holding one copy of its template per item. */
function drawnForEach(node: Obj, parent: Obj | undefined, env: Env): Obj {
  const props = isObj(node["props"]) ? node["props"] : {};
  const template = isObj(node["slots"]) ? node["slots"]["item"] : undefined;
  const raw = props["items"];
  const items = Array.isArray(raw)
    ? raw
    : exprOf(raw) === undefined
      ? undefined
      : evaluateMockBinding(exprOf(raw) as string, env.mocks, env.states, env.scope);
  if (!isObj(template) || !Array.isArray(items)) {
    return node;
  }
  // The copies stand among the parent's children: its gap and cross alignment hold between and across them.
  const pp = isObj(parent?.["props"]) ? parent["props"] : {};
  const copies = items.slice(0, MAX_DRAWN_ITEMS).map((item, index) => templateCopy(template, item, index, env));
  return {
    id: node["id"],
    type: "Flex",
    props: {
      direction: axisOf(parent),
      mainAxisSize: "min",
      ...(pp["spacing"] === undefined ? {} : { spacing: pp["spacing"] }),
      ...(pp["crossAxisAlignment"] === undefined ? {} : { crossAxisAlignment: pp["crossAxisAlignment"] }),
    },
    children: copies,
  };
}

function drawn(node: Obj, parent: Obj | undefined, env: Env): Obj {
  switch (node["type"]) {
    case "If":
      return drawnIf(node, env);
    case "Match":
      return drawnMatch(node, env);
    case "ForEach":
      return drawnForEach(node, parent, env);
    default:
      return node;
  }
}

/** The widgets of a list that are drawn: those not hidden by their `visible`. */
const keptOf = (list: readonly unknown[], env: Env): unknown[] => list.filter((c) => !isObj(c) || shown(c, env));

/** A node's slots as drawn: a hidden widget leaves its slot empty; the kept ones go through `visit`. */
function slotsOf(slots: Obj, env: Env, visit: (c: unknown) => unknown): Obj {
  const out: Obj = {};
  for (const [k, v] of Object.entries(slots)) {
    if (Array.isArray(v)) {
      out[k] = keptOf(v, env).map(visit);
    } else if (!isObj(v) || shown(v, env)) {
      out[k] = visit(v);
    }
  }
  return out;
}

/** The tree under `root` with its conditions drawn (PC-25); `root` itself when it has none. */
export function applyConditions(
  root: Node | undefined,
  mocks: SourceMocks,
  states: Readonly<Record<string, MockState>>,
  scope: DesignScope = emptyScope,
  picks: ReadonlyMap<string, BranchPick> = new Map(),
): Node | undefined {
  const env: Env = { mocks, states, scope, picks };
  const text = root === undefined ? "" : JSON.stringify(root);
  if (!/"(If|Match|ForEach)"|"visible"/.test(text)) {
    return root;
  }
  const visit = (n: unknown, parent: Obj | undefined): unknown => {
    if (!isObj(n)) {
      return n;
    }
    const node = drawn(n, parent, env);
    const only = onlyInOf(n, env);
    const out: Obj = only === undefined ? { ...node } : { ...node, onlyIn: only };
    if (Array.isArray(node["children"])) {
      out["children"] = keptOf(node["children"], env).map((c) => visit(c, node));
    }
    if (isObj(node["slots"])) {
      out["slots"] = slotsOf(node["slots"], env, (c) => visit(c, node));
    }
    return out;
  };
  return visit(root, undefined) as Node;
}
