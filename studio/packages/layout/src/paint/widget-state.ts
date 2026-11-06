// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Widget states (Flutter's `WidgetState`): a `WidgetState…` value holds one value per interaction state and a default,
 * and resolves to the first state that applies, in the order `disabled`, `error`, `dragged`, `pressed`, `hovered`,
 * `focused`, `selected` (`WidgetStateProperty.resolveWith` as the registry's `WidgetStateColor` documents it). A painted
 * screen has no pointer, so a control is only ever disabled or selected — unless Studio previews a state on it, which
 * it marks as `x-studio.state` (PC-21); the device never sees that mark.
 */

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The interaction states, in the order a `WidgetState…` value resolves them. */
export const WIDGET_STATES = ["disabled", "error", "dragged", "pressed", "hovered", "focused", "selected"] as const;
export type WidgetStateName = (typeof WIDGET_STATES)[number];

/**
 * The opacity of the state layer Material 3 lays over a control in each pointer state (`StateLayer` of the M3 spec:
 * hover 8 %, focus and press 10 %, drag 16 %).
 */
export const STATE_LAYER_OPACITY: Readonly<Partial<Record<WidgetStateName, number>>> = {
  hovered: 0.08,
  focused: 0.1,
  pressed: 0.1,
  dragged: 0.16,
};

const STATE_NAMES: ReadonlySet<string> = new Set(WIDGET_STATES);

/** Whether `value` is a `WidgetState…` value: an object of a default and per-state values, and nothing else. */
export function isStateValue(value: unknown): value is Obj {
  if (!isObj(value)) {
    return false;
  }
  const keys = Object.keys(value);
  return keys.length > 0 && keys.every((k) => k === "value" || STATE_NAMES.has(k)) && keys.some((k) => k !== "value");
}

/** The state Studio previews on a node (`x-studio.state`), if any. */
export function previewedState(node: { readonly [key: string]: unknown }): WidgetStateName | undefined {
  const studio = node["x-studio"];
  const state = isObj(studio) ? studio["state"] : undefined;
  return typeof state === "string" && STATE_NAMES.has(state) ? (state as WidgetStateName) : undefined;
}

/**
 * `value` for `states`: a `WidgetState…` value as its first state that applies and is set, else its default; a
 * `WidgetState…` value given only its `value` reads as that; any other value is returned as it is.
 */
export function resolveState(value: unknown, states: ReadonlySet<WidgetStateName>): unknown {
  if (!isObj(value) || !(isStateValue(value) || Object.keys(value).every((k) => k === "value"))) {
    return value;
  }
  const hit = WIDGET_STATES.find((s) => states.has(s) && value[s] !== undefined);
  return hit === undefined ? value["value"] : value[hit];
}
