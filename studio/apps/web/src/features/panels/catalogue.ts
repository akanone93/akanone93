// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type WidgetDescriptor, widgets } from "@plux/schema";
import { type IdSource, uuidv7 } from "../plugin-canvas/screens.ts";

/** The Insert tab's groups: Layer 1 widgets, Layer 2 components, native slots and custom widgets (WGT-030). */
export type InsertGroup = "layer1" | "layer2" | "native";

export const insertGroups: readonly InsertGroup[] = ["layer1", "layer2", "native"];

export function groupOf(d: WidgetDescriptor): InsertGroup {
  return d.layer === 1 ? "layer1" : d.layer === 2 ? "layer2" : "native";
}

/** The descriptors to offer: the built-in registry plus the app's native and custom descriptors, deduplicated by type. */
export function catalogue(extra: readonly WidgetDescriptor[] = []): readonly WidgetDescriptor[] {
  const seen = new Set<string>();
  return [...widgets, ...extra].filter((d) => d.deprecated === undefined && !seen.has(d.type) && seen.add(d.type));
}

/** Case-insensitive match on type, category and description; empty query matches everything. */
export function search(list: readonly WidgetDescriptor[], query: string): readonly WidgetDescriptor[] {
  const q = query.trim().toLowerCase();
  return q === ""
    ? list
    : list.filter((d) => [d.type, d.category, d.description].some((s) => s.toLowerCase().includes(q)));
}

const zero: Readonly<Record<string, string | number | boolean>> = {
  string: "",
  int: 0,
  number: 0,
  double: 0,
  bool: false,
};

/** A new node from the descriptor: a fresh UUIDv7 id, the descriptor's defaults, and a zero for required primitives. */
export function buildNode(
  d: WidgetDescriptor,
  newId: IdSource = uuidv7,
): { id: string; type: string; props?: Record<string, unknown> } {
  const props: Record<string, unknown> = {};
  for (const p of d.props) {
    if (p.default !== undefined) {
      props[p.name] = p.default;
    } else if (p.required && zero[p.type] !== undefined) {
      props[p.name] = zero[p.type];
    }
  }
  return Object.keys(props).length === 0 ? { id: newId(), type: d.type } : { id: newId(), type: d.type, props };
}
