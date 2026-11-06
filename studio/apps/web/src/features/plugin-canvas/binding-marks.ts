// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Binding marks (PC-23, STU-062): every widget with a prop bound to an expression carries a small badge on the canvas,
 * and the selected one lists its bindings with the value the canvas draws — a page parameter's mock, a state's
 * default, a data source's mock under its preview state — or a dash where only a device knows the value.
 */

import type { MockState } from "../preview/switches.ts";
import {
  type BoundProp,
  boundPropsOf,
  type DesignScope,
  evaluateMockBinding,
  type SourceMocks,
} from "./mock-bindings.ts";
import type { Node } from "./plugin-model.ts";

/** A bound prop of a widget with the value the canvas shows for it; undefined when only a device knows it. */
export interface BindingMark extends BoundProp {
  readonly shown?: string;
}

const MAX_SHOWN = 28;

/** A design-time value as a short text: a string quoted, a list by its length, an object by its fields. */
export function shortValue(value: unknown): string {
  if (typeof value === "string") {
    const text = value.length > MAX_SHOWN ? `${value.slice(0, MAX_SHOWN - 1)}…` : value;
    return `"${text}"`;
  }
  if (Array.isArray(value)) {
    return `[${value.length}]`;
  }
  if (typeof value === "object" && value !== null) {
    const keys = Object.keys(value);
    return `{${keys.slice(0, 3).join(", ")}${keys.length > 3 ? ", …" : ""}}`;
  }
  return String(value);
}

/** The binding marks of every widget of `screens`, by node id, each with the value its page's scope gives it. */
export function bindingMarks(
  screens: readonly { readonly id: string; readonly root?: Node | undefined }[],
  mocks: SourceMocks,
  states: Readonly<Record<string, MockState>>,
  scopes: ReadonlyMap<string, DesignScope>,
): Map<string, readonly BindingMark[]> {
  const out = new Map<string, readonly BindingMark[]>();
  for (const s of screens) {
    const scope = scopes.get(s.id);
    for (const [id, list] of boundPropsOf(s.root)) {
      out.set(
        id,
        list.map((b) => {
          const v = evaluateMockBinding(b.expr, mocks, states, scope);
          return v === undefined || v === null ? b : { ...b, shown: shortValue(v) };
        }),
      );
    }
  }
  return out;
}
