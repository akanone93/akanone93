// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * What Material 3's controls paint (STU-005): buttons, icon buttons, floating action buttons, chips, checkboxes,
 * radios, switches, sliders and progress indicators, with the defaults of Flutter's `_*DefaultsM3` (colour roles,
 * shapes, elevation). A control with no action is disabled, as on the device, where Flutter disables a button whose
 * `onPressed` is null: its container takes `onSurface` at 12 % and its content at 38 %. The control's laid-out box is
 * its tap target; the visible control is centred in it.
 */

import type { ColorRole } from "../material-color/scheme.ts";
import {
  type BoxPaint,
  elevationShadows,
  type PaintTheme,
  type Radii,
  type Rect,
  type Rgba,
  resolveColor,
  roleColor,
  shapePaint,
} from "./paint.ts";
import { previewedState, resolveState, STATE_LAYER_OPACITY, type WidgetStateName } from "./widget-state.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

/** A node as the control painters read it: its type, props and the actions it handles. */
export interface ControlNode {
  readonly type?: string;
  readonly props?: Obj;
  readonly events?: unknown;
  readonly slots?: unknown;
  /** Editor-only marks; `state` is the interaction state Studio previews (PC-21). */
  readonly "x-studio"?: unknown;
}

/** The actions that enable each control; a control handling none of them is disabled. */
const ACTIONS: Readonly<Record<string, readonly string[]>> = {
  FilledButton: ["onPressed", "onLongPress"],
  FilledTonalButton: ["onPressed", "onLongPress"],
  ElevatedButton: ["onPressed", "onLongPress"],
  OutlinedButton: ["onPressed", "onLongPress"],
  TextButton: ["onPressed", "onLongPress"],
  IconButton: ["onPressed", "onLongPress"],
  FloatingActionButton: ["onPressed"],
  ActionChip: ["onPressed"],
  FilterChip: ["onSelected"],
  ChoiceChip: ["onSelected"],
  InputChip: ["onPressed", "onSelected", "onDeleted"],
  Checkbox: ["onChanged"],
  Radio: ["onChanged"],
  Switch: ["onChanged"],
  Slider: ["onChanged"],
};

/** Whether `node` is a control with no action: Flutter draws it disabled. */
export function disabledControl(node: ControlNode): boolean {
  if (previewedState(node as Obj) === "disabled") {
    return true;
  }
  const actions = ACTIONS[node.type ?? ""];
  if (actions === undefined) {
    return false;
  }
  const events = isObj(node.events) ? node.events : {};
  return !actions.some((a) => events[a] !== undefined && events[a] !== null);
}

/** `rgba` with alpha `a` (0–255). */
export const withAlpha = (rgba: Rgba, a: number): Rgba => ((rgba & 0xffffff00) | a) >>> 0;
const DISABLED_CONTAINER = 0x1f;
const DISABLED_CONTENT = 0x61;

/** A box of `w` by `h` centred in `r`, no larger than it. */
function centred(r: Rect, w: number, h: number): Rect {
  const cw = Math.min(w, r.w);
  const ch = Math.min(h, r.h);
  return { x: r.x + (r.w - cw) / 2, y: r.y + (r.h - ch) / 2, w: cw, h: ch };
}

const pill = (b: Rect): Radii => {
  const radius = b.h / 2;
  return [radius, radius, radius, radius];
};
const corners = (radius: number): Radii => [radius, radius, radius, radius];

/** The colour of the content (label and icon) of a control, by its state. */
export interface Foreground {
  readonly text?: Rgba;
  readonly icon?: Rgba;
  readonly iconSize?: number;
}

const BUTTON_CONTENT: Readonly<Record<string, ColorRole>> = {
  FilledButton: "onPrimary",
  FilledTonalButton: "onSecondaryContainer",
  ElevatedButton: "primary",
  OutlinedButton: "primary",
  TextButton: "primary",
};

/** Whether a chip is selected: a filter, choice or input chip's `selected`. */
const selectedChip = (node: ControlNode) => node.props?.["selected"] === true;

/**
 * The colours a control gives the texts and icons inside it (its `DefaultTextStyle` and `IconTheme` in Flutter), and
 * its icons' size; undefined when it gives none.
 */
export function controlForeground(node: ControlNode, theme: PaintTheme): Foreground | undefined {
  const type = node.type ?? "";
  const disabled = disabledControl(node);
  const role = (r: ColorRole) =>
    disabled ? withAlpha(roleColor(theme, "onSurface"), DISABLED_CONTENT) : roleColor(theme, r);
  const button = BUTTON_CONTENT[type];
  if (button !== undefined) {
    return styledForeground(node, theme, { text: role(button), icon: role(button), iconSize: 18 });
  }
  switch (type) {
    case "IconButton":
      return { icon: role("onSurfaceVariant"), iconSize: 24 };
    case "FloatingActionButton":
      return { text: role("onPrimaryContainer"), icon: role("onPrimaryContainer"), iconSize: 24 };
    case "ActionChip":
      return { text: role("onSurface"), icon: role("primary"), iconSize: 18 };
    case "Chip":
    case "FilterChip":
    case "ChoiceChip":
    case "InputChip": {
      const on = selectedChip(node) ? role("onSecondaryContainer") : role("onSurfaceVariant");
      return { text: on, icon: on, iconSize: 18 };
    }
    default:
      return undefined;
  }
}

/** The states a control is in on the canvas: disabled without an action, selected, and the one Studio previews. */
export function controlStates(node: ControlNode): ReadonlySet<WidgetStateName> {
  const states = new Set<WidgetStateName>();
  if (disabledControl(node)) {
    states.add("disabled");
  }
  if (node.props?.["selected"] === true) {
    states.add("selected");
  }
  const shown = previewedState(node as Obj);
  if (shown !== undefined) {
    states.add(shown);
  }
  return states;
}

const styleOf = (node: ControlNode): Obj => (isObj(node.props?.["style"]) ? (node.props["style"] as Obj) : {});

/** A control's content colours with its `style`'s foreground and icon colours and icon size for its states. */
function styledForeground(node: ControlNode, theme: PaintTheme, base: Foreground): Foreground {
  const style = styleOf(node);
  const states = controlStates(node);
  const fg = resolveColor(resolveState(style["foregroundColor"], states), theme);
  const icon = resolveColor(resolveState(style["iconColor"], states), theme) ?? fg;
  const size = num(resolveState(style["iconSize"], states));
  return {
    ...base,
    ...(fg === undefined ? {} : { text: fg }),
    ...(icon === undefined ? {} : { icon }),
    ...(size === undefined ? {} : { iconSize: size }),
  };
}

/**
 * A button's own look from its `style` (a `ButtonStyle`), over the defaults of its kind: background, outline, shape
 * and elevation for its states, and the state layer of a previewed pointer state — the overlay colour, else the
 * content colour, at the opacity Material 3 gives the state.
 */
function styledButton(node: ControlNode, theme: PaintTheme, box: BoxPaint, painted: boolean): BoxPaint[] {
  const style = styleOf(node);
  if (Object.keys(style).length === 0 && previewedState(node as Obj) === undefined) {
    return painted ? [box] : [];
  }
  const states = controlStates(node);
  const fill = resolveColor(resolveState(style["backgroundColor"], states), theme);
  const shape = resolveState(style["shape"], states);
  const outline = shapePaint({ side: resolveState(style["side"], states) }, box, theme, pill(box)).border;
  const elevation = num(resolveState(style["elevation"], states));
  const own: BoxPaint = {
    ...box,
    ...(fill === undefined ? {} : { fill }),
    ...(shape === undefined ? {} : shapePaint(shape, box, theme, box.radii ?? pill(box))),
    ...(outline === undefined ? {} : { border: outline }),
    ...(elevation === undefined ? {} : { shadows: elevationShadows(elevation, roleColor(theme, "shadow")) }),
  };
  const layer = stateLayer(node, theme, own);
  return layer === undefined ? [own] : [own, layer];
}

/** The state layer over a control in the pointer state Studio previews, if any. */
function stateLayer(node: ControlNode, theme: PaintTheme, box: BoxPaint): BoxPaint | undefined {
  const shown = previewedState(node as Obj);
  const opacity = shown === undefined ? undefined : STATE_LAYER_OPACITY[shown];
  if (opacity === undefined) {
    return undefined;
  }
  const states = controlStates(node);
  const overlay = resolveColor(resolveState(styleOf(node)["overlayColor"], states), theme);
  const base = overlay ?? controlForeground(node, theme)?.text ?? roleColor(theme, "primary");
  const { fill: _fill, gradient: _gradient, shadows: _shadows, border: _border, ...shape } = box;
  return { ...shape, fill: withAlpha(base, Math.round(255 * opacity)) };
}

/** A Material 3 common button: a pill 40 high in its tap target, then its own `style`. */
function button(kind: string, node: ControlNode, r: Rect, theme: PaintTheme): BoxPaint[] {
  const b = centred(r, r.w, 40);
  const [box] = defaultButton(kind, node, b, theme);
  return styledButton(node, theme, box ?? { ...b, radii: pill(b) }, box !== undefined);
}

/** A common button's default look by its kind. */
function defaultButton(kind: string, node: ControlNode, b: Rect, theme: PaintTheme): BoxPaint[] {
  const shape = { ...b, radii: pill(b) };
  if (disabledControl(node)) {
    const off = withAlpha(roleColor(theme, "onSurface"), DISABLED_CONTAINER);
    if (kind === "OutlinedButton") {
      return [{ ...shape, border: { kind: "all", side: { color: off, width: 1 } } }];
    }
    return kind === "TextButton" ? [] : [{ ...shape, fill: off }];
  }
  switch (kind) {
    case "FilledButton":
      return [{ ...shape, fill: roleColor(theme, "primary") }];
    case "FilledTonalButton":
      return [{ ...shape, fill: roleColor(theme, "secondaryContainer") }];
    case "ElevatedButton":
      return [
        {
          ...shape,
          fill: roleColor(theme, "surfaceContainerLow"),
          shadows: elevationShadows(1, roleColor(theme, "shadow")),
        },
      ];
    case "OutlinedButton":
      return [{ ...shape, border: { kind: "all", side: { color: roleColor(theme, "outline"), width: 1 } } }];
    default:
      return [];
  }
}

/** A floating action button: its size and corner by kind, in `primaryContainer`, at elevation 6. */
function fab(node: ControlNode, r: Rect, theme: PaintTheme): BoxPaint[] {
  const kind = node.props?.["mini"] === true ? "small" : node.props?.["kind"];
  const [w, h, radius] =
    kind === "small"
      ? [40, 40, 12]
      : kind === "large"
        ? [96, 96, 28]
        : kind === "extended"
          ? [r.w, 56, 16]
          : [56, 56, 16];
  const b = centred(r, w, h);
  return [
    {
      ...b,
      radii: corners(radius),
      fill: roleColor(theme, "primaryContainer"),
      shadows: elevationShadows(num(node.props?.["elevation"]) ?? 6, roleColor(theme, "shadow")),
    },
  ];
}

/** A chip: 32 high, corners of 8; outlined, or filled in `secondaryContainer` when selected. */
function chip(node: ControlNode, r: Rect, theme: PaintTheme): BoxPaint[] {
  const b = centred(r, r.w, 32);
  const shape = { ...b, radii: corners(8) };
  if (selectedChip(node)) {
    return [{ ...shape, fill: roleColor(theme, "secondaryContainer") }];
  }
  const side = disabledControl(node)
    ? withAlpha(roleColor(theme, "onSurface"), DISABLED_CONTAINER)
    : roleColor(theme, "outlineVariant");
  return [{ ...shape, border: { kind: "all", side: { color: side, width: 1 } } }];
}

/** The colour of a selection control: `primary` when on, `onSurfaceVariant` when off, `onSurface` at 38 % disabled. */
function selection(node: ControlNode, on: boolean, theme: PaintTheme): Rgba {
  if (disabledControl(node)) {
    return withAlpha(roleColor(theme, "onSurface"), DISABLED_CONTENT);
  }
  return roleColor(theme, on ? "primary" : "onSurfaceVariant");
}

/** A checkbox: an 18 pixel square with corners of 2, outlined off, filled on; its check is an icon over it. */
function checkbox(node: ControlNode, r: Rect, theme: PaintTheme): BoxPaint[] {
  const value = node.props?.["value"];
  const on = value === true || (value === null && node.props?.["tristate"] === true);
  const b = { ...centred(r, 18, 18), radii: corners(2) };
  const color = selection(node, on, theme);
  return on ? [{ ...b, fill: color }] : [{ ...b, border: { kind: "all", side: { color, width: 2 } } }];
}

/** A radio: a 20 pixel ring, with a 10 pixel dot when its value is its group's. */
function radio(node: ControlNode, r: Rect, theme: PaintTheme): BoxPaint[] {
  const p = node.props ?? {};
  const on = p["value"] !== undefined && JSON.stringify(p["value"]) === JSON.stringify(p["groupValue"]);
  const color = selection(node, on, theme);
  const ring = { ...centred(r, 20, 20), oval: true, border: { kind: "all" as const, side: { color, width: 2 } } };
  return on ? [ring, { ...centred(r, 10, 10), oval: true, fill: color }] : [ring];
}

/** A switch: a 52 by 32 track, a 24 pixel thumb at the end when on, a 16 pixel thumb at the start when off. */
function switchPaint(node: ControlNode, r: Rect, theme: PaintTheme): BoxPaint[] {
  const on = node.props?.["value"] === true;
  const track = centred(r, 52, 32);
  const off = disabledControl(node);
  const dim = (c: Rgba) => (off ? withAlpha(c, DISABLED_CONTAINER) : c);
  const thumbSize = on ? 24 : 16;
  const cx = on ? track.x + track.w - 16 : track.x + 16;
  const thumb = {
    x: cx - thumbSize / 2,
    y: track.y + (track.h - thumbSize) / 2,
    w: thumbSize,
    h: thumbSize,
    oval: true,
  };
  if (on) {
    return [
      { ...track, radii: pill(track), fill: dim(roleColor(theme, "primary")) },
      { ...thumb, fill: off ? roleColor(theme, "surface") : roleColor(theme, "onPrimary") },
    ];
  }
  const outline = off ? withAlpha(roleColor(theme, "onSurface"), DISABLED_CONTAINER) : roleColor(theme, "outline");
  return [
    {
      ...track,
      radii: pill(track),
      fill: dim(roleColor(theme, "surfaceContainerHighest")),
      border: { kind: "all", side: { color: outline, width: 2 } },
    },
    { ...thumb, fill: off ? withAlpha(roleColor(theme, "onSurface"), DISABLED_CONTENT) : roleColor(theme, "outline") },
  ];
}

/** A slider: a 4 pixel track inset by its overlay radius, active up to the value, and a 20 pixel thumb. */
function slider(node: ControlNode, r: Rect, theme: PaintTheme): BoxPaint[] {
  const p = node.props ?? {};
  const min = num(p["min"]) ?? 0;
  const max = num(p["max"]) ?? 1;
  const value = Math.min(max, Math.max(min, num(p["value"]) ?? min));
  const t = max > min ? (value - min) / (max - min) : 0;
  const left = r.x + 20;
  const width = Math.max(0, r.w - 40);
  const y = r.y + r.h / 2;
  const at = left + width * t;
  const off = disabledControl(node);
  const active = off ? withAlpha(roleColor(theme, "onSurface"), DISABLED_CONTENT) : roleColor(theme, "primary");
  const inactive = off
    ? withAlpha(roleColor(theme, "onSurface"), DISABLED_CONTAINER)
    : roleColor(theme, "surfaceContainerHighest");
  return [
    { x: left, y: y - 2, w: width, h: 4, radii: corners(2), fill: inactive },
    { x: left, y: y - 2, w: at - left, h: 4, radii: corners(2), fill: active },
    { x: at - 10, y: y - 10, w: 20, h: 20, oval: true, fill: active },
  ];
}

/** A linear progress indicator: its track in `secondaryContainer`, the value (or a moment of the animation) in primary. */
function linearProgress(node: ControlNode, r: Rect, theme: PaintTheme): BoxPaint[] {
  const t = Math.min(1, Math.max(0, num(node.props?.["value"]) ?? 0.4));
  return [
    { ...r, fill: roleColor(theme, "secondaryContainer") },
    { x: r.x, y: r.y, w: r.w * t, h: r.h, fill: roleColor(theme, "primary") },
  ];
}

/**
 * A circular progress indicator: with a value, an arc of its stroke width in primary from 12 o'clock; without one, a
 * moment of its animation, a quarter of the ring.
 */
function circularProgress(node: ControlNode, r: Rect, theme: PaintTheme): BoxPaint[] {
  const size = Math.min(r.w, r.h);
  const stroke = num(node.props?.["strokeWidth"]) ?? 4;
  const value = num(node.props?.["value"]);
  const sweep = value === undefined ? 90 : Math.min(1, Math.max(0, value)) * 360;
  return [
    {
      ...centred(r, size, size),
      oval: true,
      fill: roleColor(theme, "primary"),
      arc: { start: -90, sweep, width: stroke },
    },
  ];
}

/**
 * A badge in `error` at the top end of what it badges, where Flutter's `_Badge` puts it: a 6 pixel dot, or with a
 * label a pill 16 high offset by (4, 4) and lifted by half its height. The label's text is not laid out yet; its pill
 * is drawn at its smallest width.
 */
function badge(node: ControlNode, r: Rect, theme: PaintTheme): BoxPaint[] {
  if (node.props?.["isLabelVisible"] === false) {
    return [];
  }
  const label = isObj(node.slots) && node.slots["label"] !== undefined;
  const fill = roleColor(theme, "error");
  if (!label) {
    const size = num(node.props?.["smallSize"]) ?? 6;
    return [{ x: r.x + r.w - size, y: r.y, w: size, h: size, oval: true, fill, foreground: true }];
  }
  const size = num(node.props?.["largeSize"]) ?? 16;
  const b = { x: r.x + r.w - 12, y: r.y + 4 - size / 2, w: size, h: size };
  return [{ ...b, radii: pill(b), fill, foreground: true }];
}

type ControlPainter = (node: ControlNode, r: Rect, theme: PaintTheme) => BoxPaint[];

/** The painters of the controls, by widget type. */
export const controlPainters: ReadonlyMap<string, ControlPainter> = new Map<string, ControlPainter>([
  ...Object.keys(BUTTON_CONTENT).map((k): [string, ControlPainter] => [k, (n, r, t) => button(k, n, r, t)]),
  ["IconButton", () => []],
  ["FloatingActionButton", fab],
  ["Chip", chip],
  ["ActionChip", chip],
  ["FilterChip", chip],
  ["ChoiceChip", chip],
  ["InputChip", chip],
  ["Checkbox", checkbox],
  ["Radio", radio],
  ["Switch", switchPaint],
  ["Slider", slider],
  ["LinearProgressIndicator", linearProgress],
  ["CircularProgressIndicator", circularProgress],
  ["Badge", badge],
]);

/** The check a checked checkbox draws over its box, as an icon (`check`, or `remove` for a tristate's null). */
export function checkboxMark(node: ControlNode, theme: PaintTheme): { name: string; color: Rgba } | undefined {
  if (node.type !== "Checkbox") {
    return undefined;
  }
  const value = node.props?.["value"];
  if (value !== true && !(value === null && node.props?.["tristate"] === true)) {
    return undefined;
  }
  const color = disabledControl(node) ? roleColor(theme, "surface") : roleColor(theme, "onPrimary");
  return { name: value === true ? "check" : "remove", color };
}
