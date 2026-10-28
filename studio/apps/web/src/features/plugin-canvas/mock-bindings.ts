// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { MockState } from "../preview/switches.ts";
import type { Node } from "./plugin-model.ts";

/** Design-time data for the canvas (DAT-080): a source's mock under the state the preview switch selects. */

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** A data source's mock, by source ID. */
export type SourceMocks = ReadonlyMap<string, unknown>;

/** `data.<source>.<value|loading|error>` then member steps, optionally `?? <JSON literal>`. */
const binding = /^\s*data\.(\w+)\.(value|loading|error)((?:\??\.\w+|\[\d+\])*)(?:\s*\?\?\s*(.+?))?\s*$/;
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
): unknown {
  const match = binding.exec(expr);
  const id = match?.[1];
  if (match === null || id === undefined || !mocks.has(id)) {
    return undefined;
  }
  let current: unknown = stateOf(mocks.get(id), states[id] ?? "success")[match[2] as "value"];
  for (const s of (match[3] ?? "").matchAll(step)) {
    const key = s[1] ?? Number(s[2]);
    current = Array.isArray(current) && typeof key === "number" ? current[key] : isObj(current) ? current[key] : null;
  }
  return current ?? literal(match[4]) ?? null;
}

/** The mock value of a `{"$expr": "…"}` prop, `undefined` when `value` is not a binding this module can read. */
function resolved(value: unknown, mocks: SourceMocks, states: Readonly<Record<string, MockState>>): unknown {
  const expr = isObj(value) && Object.keys(value).length === 1 ? value["$expr"] : undefined;
  return typeof expr === "string" ? evaluateMockBinding(expr, mocks, states) : undefined;
}

function bindValue(value: unknown, mocks: SourceMocks, states: Readonly<Record<string, MockState>>): unknown {
  if (Array.isArray(value)) {
    return value.map((v) => {
      const bound = resolved(v, mocks, states);
      return bound === undefined ? bindValue(v, mocks, states) : bound;
    });
  }
  if (!isObj(value)) {
    return value;
  }
  const out: Obj = {};
  for (const [key, child] of Object.entries(value)) {
    const bound = resolved(child, mocks, states);
    if (bound === undefined) {
      out[key] = bindValue(child, mocks, states);
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
): Node | undefined {
  return root === undefined || mocks.size === 0 ? root : (bindValue(root, mocks, states) as Node);
}
