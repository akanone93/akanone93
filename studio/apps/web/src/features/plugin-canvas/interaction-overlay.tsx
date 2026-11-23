// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Guides } from "@plux/canvas";
import { type CSSProperties, type ReactNode, useEffect, useRef } from "react";
import type { NodeProblem } from "../problems/problem-nodes.ts";
import { A11Y_TEXT_ZOOM, badgeAt, keepApart, type Label, tagAbove, tagBelow } from "./a11y-labels.ts";
import { type LowContrast, smallTarget } from "./a11y-marks.ts";
import type { BindingMark } from "./binding-marks.ts";
import type { InstanceMark } from "./component-instances.ts";
import type { L10nOverlay } from "./l10n-marks.ts";
import { cornersShown, tagClear } from "./overlay-layout.ts";
import type { HandlerStep } from "./quick-actions.ts";
import type { SizesOverlay } from "./responsive.ts";
import type { CostsOverlay, CostWords, ScreenCost } from "./screen-costs.ts";

/** What the overlay draws of component instances (PC-37). */
export interface InstancesOverlay {
  readonly marks: ReadonlyMap<string, InstanceMark>;
  readonly tag: (mark: InstanceMark) => string;
  readonly isolated: string | undefined;
}

import type { Box } from "./select/geometry.ts";
import { HANDLES, handlePoint } from "./select/handles.ts";
import { type OverlayState, type OverlayStore, type RefusedDrop, useOverlayState } from "./select/overlay-store.ts";
import type { Selection } from "./select/selection.ts";
import type { CanvasSurface } from "./surface.ts";
import "./interaction-overlay.css";

/** What the overlay needs to know about the nodes it marks. */
export interface OverlayNodes {
  /** The world box of a node (a screen's root: its device frame), or undefined while it is not laid out. */
  readonly boxOf: (id: string) => Box | undefined;
  /** The name shown on a node's tag: its layer name, else its widget type, else the screen's title for a root. */
  readonly nameOf: (id: string) => string;
  /** Whether a node is a screen's root: it is marked as the screen. */
  readonly isScreen: (id: string) => boolean;
  /** Whether a node can be resized: it has `width` and `height`, and is not locked. */
  readonly resizable: (id: string) => boolean;
  /** The container the selection is inside (its scope), drawn faintly; null at the top level. */
  readonly scope: string | null;
  /** The bottom of a screen's content below its frame (its overflow band, STU-056), by its root; none when it fits. */
  readonly overflowBottom?: (root: string) => number | undefined;
  /** The device frame's corner radius in world units (PC-11); a generic phone's when absent. */
  readonly frameRadius?: number;
  /** A node's children, in order, whose padding and gaps the selection shows (PC-16). */
  readonly childrenOf?: (id: string) => readonly string[];
  /** Whether the space around a node's single child is its padding (not an alignment's free space). */
  readonly padded?: (id: string) => boolean;
  /** The problems pinned to each node, the most severe first (PC-16). */
  readonly problems?: ReadonlyMap<string, readonly NodeProblem[]>;
  /** The props of each node bound to expressions, with the values the canvas shows (PC-23). */
  readonly bindings?: ReadonlyMap<string, readonly BindingMark[]>;
  /** The steps each node's events run (PC-27). */
  readonly events?: ReadonlyMap<string, readonly HandlerStep[]>;
  /** The widgets of other screens that share a node's hero tag (PC-29). */
  readonly heroes?: (id: string) => readonly string[];
  /** The Accessibility view's marks, when it is on (PC-31). */
  readonly a11y?: A11yMarks;
  /** The Localisation view's marks, when it is on (PC-32), with their words. */
  readonly l10n?: L10nOverlay;
  /** The Size classes view's marks, when it is on (PC-33). */
  readonly sizes?: SizesOverlay;
  /** What each screen costs, by its root's id, with its words (PC-34). */
  readonly costs?: CostsOverlay;
  /** The component instances, their words, and the one isolated (PC-37). */
  readonly instances?: InstancesOverlay;
  /**
   * Every widget's event, binding and component badges (#216); without it they show only on the hovered and selected
   * widgets, so the screens stay clean and a render does not walk every widget. Problem dots always show.
   */
  readonly allBadges?: boolean;
  /** Why a drop is refused, in a few words (PC-20), and what a refused drop says once let go. */
  readonly dropText?: {
    readonly refused: (r: RefusedDrop, parent: string) => string;
    readonly notice: (reason: string) => string;
  };
  /** The words of the marks. */
  readonly a11yText?: {
    readonly target: (w: number, h: number, min: number) => string;
    readonly contrast: (r: string, n: string) => string;
  };
}

/** What the Accessibility view marks (PC-31): focus order, the widgets that need a minimum target, low contrast. */
export interface A11yMarks {
  readonly order: ReadonlyMap<string, number>;
  readonly targets: ReadonlySet<string>;
  readonly low: ReadonlyMap<string, LowContrast>;
  /** The minimum touch target of the previewed platform: 48 on Android, 44 on iOS. */
  readonly min: number;
}

interface View {
  readonly x: number;
  readonly y: number;
  readonly zoom: number;
}

/** A world box in canvas pixels, as the CSS variables the overlay's classes place it by. */
function place(b: Box, v: View): CSSProperties {
  return {
    "--x": (b.l - v.x) * v.zoom,
    "--y": (b.t - v.y) * v.zoom,
    "--w": Math.max(0, (b.r - b.l) * v.zoom),
    "--h": Math.max(0, (b.b - b.t) * v.zoom),
  } as CSSProperties;
}

const point = (x: number, y: number, v: View): CSSProperties =>
  ({ "--x": (x - v.x) * v.zoom, "--y": (y - v.y) * v.zoom }) as CSSProperties;

/** The size of a box in whole world units, as design tools label it. */
const sizeText = (b: Box): string => `${Math.round(b.r - b.l)} × ${Math.round(b.b - b.t)}`;

function Tag({ box, view, text, kind }: { box: Box; view: View; text: string; kind: string }): ReactNode {
  // A drop's tag sits at the box's top right, clear of the selection's name at its top left.
  const end = kind === "drop" || kind === "refused";
  return (
    <span
      className="pc-tag"
      data-kind={kind}
      data-end={end || undefined}
      style={point(end ? box.r : box.l, box.t, view)}
    >
      {text}
    </span>
  );
}

/** The corner radius of a device frame without chrome, in world units, as the canvas draws it. */
const FRAME_RADIUS = 32;

/** A screen's outline radius at the view's zoom: its device's corners. */
const frameCorner = (nodes: OverlayNodes, view: View): number => (nodes.frameRadius ?? FRAME_RADIUS) * view.zoom;

/** A band of padding or gap, in world units, with the size it shows. */
interface Band {
  readonly box: Box;
  readonly value: number;
  readonly kind: "padding" | "gap";
}

const band = (box: Box, value: number, kind: Band["kind"]): Band[] =>
  value >= 0.5 && box.r > box.l && box.b > box.t ? [{ box, value, kind }] : [];

/** The padding around a single child: the four bands between the box and the child's. */
function paddingBands(box: Box, kid: Box): Band[] {
  if (kid.l < box.l - 0.5 || kid.t < box.t - 0.5 || kid.r > box.r + 0.5 || kid.b > box.b + 0.5) {
    return [];
  }
  return [
    ...band({ l: box.l, t: box.t, r: box.r, b: kid.t }, kid.t - box.t, "padding"),
    ...band({ l: box.l, t: kid.b, r: box.r, b: box.b }, box.b - kid.b, "padding"),
    ...band({ l: box.l, t: kid.t, r: kid.l, b: kid.b }, kid.l - box.l, "padding"),
    ...band({ l: kid.r, t: kid.t, r: box.r, b: kid.b }, box.r - kid.r, "padding"),
  ];
}

/** Whether `kids`, in order, follow one another along an axis without overlapping. */
function inLine(kids: readonly Box[], axis: "x" | "y"): boolean {
  return kids.every((k, i) => {
    const prev = kids[i - 1];
    return prev === undefined || (axis === "x" ? k.l >= prev.r - 0.5 : k.t >= prev.b - 0.5);
  });
}

/** The gaps between children laid out in a row or a column. */
function gapBands(kids: readonly Box[]): Band[] {
  const byX = [...kids].sort((a, b) => a.l - b.l);
  const byY = [...kids].sort((a, b) => a.t - b.t);
  const axis = inLine(byY, "y") ? "y" : inLine(byX, "x") ? "x" : undefined;
  if (axis === undefined) {
    return [];
  }
  const sorted = axis === "y" ? byY : byX;
  return sorted.flatMap((k, i) => {
    const next = sorted[i + 1];
    if (next === undefined) {
      return [];
    }
    return axis === "y"
      ? band({ l: Math.min(k.l, next.l), t: k.b, r: Math.max(k.r, next.r), b: next.t }, next.t - k.b, "gap")
      : band({ l: k.r, t: Math.min(k.t, next.t), r: next.l, b: Math.max(k.b, next.b) }, next.l - k.r, "gap");
  });
}

/** The spacing a selected widget shows (PC-16): its padding around one child, or the gaps between several. */
export function spacingBands(box: Box, kids: readonly Box[], padded = true): Band[] {
  if (kids.length === 1) {
    return padded ? paddingBands(box, kids[0] as Box) : [];
  }
  return kids.length > 1 ? gapBands(kids) : [];
}

function Spacing({ id, box, view, nodes }: { id: string; box: Box; view: View; nodes: OverlayNodes }): ReactNode {
  const kids = (nodes.childrenOf?.(id) ?? []).flatMap((k) => nodes.boxOf(k) ?? []);
  return spacingBands(box, kids, nodes.padded?.(id) ?? true).map((b, i) => {
    const big = (b.box.r - b.box.l) * view.zoom >= 14 && (b.box.b - b.box.t) * view.zoom >= 10;
    return (
      // biome-ignore lint/suspicious/noArrayIndexKey: the bands of one selection, in a fixed order
      <div key={i} className="pc-spacing" data-kind={b.kind} style={place(b.box, view)}>
        {big && <span className="pc-spacing-value">{Math.round(b.value)}</span>}
      </div>
    );
  });
}

/** The badges of one widget: its box, problem dot, bindings and event steps, or none when it has nothing to show. */
function badgeMarkOf(id: string, nodes: OverlayNodes, shows: boolean) {
  const box = nodes.boxOf(id);
  const severity = nodes.problems?.get(id)?.[0]?.severity;
  const dot = severity === "error" || severity === "warning" ? severity : undefined;
  // A screen shows only its dot; another widget its events and bindings too while they are shown (#216).
  const own = shows && !nodes.isScreen(id);
  const bound = own ? (nodes.bindings?.get(id) ?? []) : [];
  const steps = own ? (nodes.events?.get(id) ?? []) : [];
  return box === undefined || (dot === undefined && bound.length === 0 && steps.length === 0)
    ? []
    : [{ id, box, dot, bound, steps }];
}

/**
 * The widgets with corner badges. Problem dots always show; events and bindings show on every widget only with
 * `allBadges`, else on the hovered and selected widgets alone, and only those are walked (#216).
 */
function badgeMarks(nodes: OverlayNodes, focus: ReadonlySet<string>) {
  const all = nodes.allBadges === true;
  const badged = all ? [...(nodes.bindings?.keys() ?? []), ...(nodes.events?.keys() ?? [])] : [...focus];
  return [...new Set([...(nodes.problems?.keys() ?? []), ...badged])].flatMap((id) =>
    badgeMarkOf(id, nodes, all || focus.has(id)),
  );
}

/**
 * The badges on a widget's top-end corner, as one cluster (C4): the steps its events run (amber, PC-27), the props it
 * binds (teal, PC-23) and its problem (a red or amber dot, PC-16), the dot on the corner itself. Where nested widgets
 * share a corner, only the innermost cluster is drawn, besides the hovered and selected widgets' (`cornersShown`).
 */
function CornerBadges({
  view,
  nodes,
  focus,
}: {
  view: View;
  nodes: OverlayNodes;
  focus: ReadonlySet<string>;
}): ReactNode {
  const marked = badgeMarks(nodes, focus);
  const shown = cornersShown(
    marked.map((m) => ({
      id: m.id,
      x: (m.box.r - view.x) * view.zoom,
      y: (m.box.t - view.y) * view.zoom,
      area: (m.box.r - m.box.l) * (m.box.b - m.box.t) * view.zoom * view.zoom,
    })),
    focus,
  );
  return marked
    .filter((m) => shown.has(m.id))
    .map((m) => (
      <span key={m.id} className="pc-badges" style={point(m.box.r, m.box.t, view)}>
        {m.steps.length > 0 && <span className="pc-event-badge">{m.steps.length > 1 ? m.steps.length : ""}</span>}
        {m.bound.length > 0 && <span className="pc-bind-badge">{m.bound.length > 1 ? m.bound.length : ""}</span>}
        {m.dot !== undefined && <span className="pc-problem-dot" data-severity={m.dot} />}
      </span>
    ));
}

const MAX_BINDINGS = 3;

/** The selected widget's event handlers: each event and the steps it runs. */
function EventTag({ id, nodes }: { id: string; nodes: OverlayNodes }): ReactNode {
  const steps = nodes.events?.get(id) ?? [];
  if (steps.length === 0) {
    return null;
  }
  return (
    <span className="pc-event-tag">
      {steps.slice(0, MAX_BINDINGS).map((s) => (
        <span key={`${s.event}:${s.index}`} className="pc-bind-line">
          <span className="pc-bind-prop">{s.event}</span>
          <span className="pc-event-action">{s.action}</span>
          {s.detail !== undefined && <span className="pc-bind-value">{s.detail}</span>}
        </span>
      ))}
      {steps.length > MAX_BINDINGS && <span className="pc-bind-more">+{steps.length - MAX_BINDINGS}</span>}
    </span>
  );
}

/** The selected widget's bindings: each prop, its expression and the value drawn. */
function BindingTag({ id, nodes }: { id: string; nodes: OverlayNodes }): ReactNode {
  const list = nodes.bindings?.get(id) ?? [];
  if (list.length === 0) {
    return null;
  }
  return (
    <span className="pc-bind-tag">
      {list.slice(0, MAX_BINDINGS).map((b) => (
        <span key={b.prop} className="pc-bind-line">
          <span className="pc-bind-prop">{b.prop}</span>
          <span className="pc-bind-expr">{b.expr}</span>
          <span className="pc-bind-value" data-unknown={b.shown === undefined || undefined}>
            {b.shown ?? "—"}
          </span>
        </span>
      ))}
      {list.length > MAX_BINDINGS && <span className="pc-bind-more">+{list.length - MAX_BINDINGS}</span>}
    </span>
  );
}

/** One label of the Accessibility view, as drawn: its key, class, kind, words and place. */
interface A11yLabel {
  readonly key: string;
  readonly className: string;
  readonly kind?: string;
  readonly text: string;
  readonly style: CSSProperties;
}

/** A world point in canvas pixels. */
const onScreen = (x: number, y: number, v: View) => ({ x: (x - v.x) * v.zoom, y: (y - v.y) * v.zoom });

/** The findings' labels, low contrast first, and the small targets' outlines, which are always drawn. */
function findingLabels(view: View, nodes: OverlayNodes, a: A11yMarks, outlines: ReactNode[]): Label<A11yLabel>[] {
  const labels: Label<A11yLabel>[] = [];
  for (const [id, c] of a.low) {
    const b = nodes.boxOf(id);
    if (b !== undefined) {
      const text = nodes.a11yText?.contrast(c.ratio.toFixed(1), String(c.needed)) ?? "";
      const p = onScreen(b.r, b.t, view);
      const style = point(b.r, b.t, view);
      labels.push({
        rect: tagAbove(p.x, p.y, text),
        mark: { key: `c${id}`, className: "pc-a11y-tag", kind: "contrast", text, style },
      });
    }
  }
  for (const id of a.targets) {
    const b = nodes.boxOf(id);
    const size = b === undefined ? undefined : { w: b.r - b.l, h: b.b - b.t };
    if (b !== undefined && size !== undefined && smallTarget(size, a.min)) {
      outlines.push(<div key={`t${id}`} className="pc-outline" data-kind="a11y-target" style={place(b, view)} />);
      const text = nodes.a11yText?.target(Math.round(size.w), Math.round(size.h), a.min) ?? "";
      const p = onScreen(b.l, b.b, view);
      labels.push({
        rect: tagBelow(p.x, p.y, text),
        mark: { key: `l${id}`, className: "pc-a11y-tag", text, style: point(b.l, b.b, view) },
      });
    }
  }
  return labels;
}

/**
 * The Accessibility view (PC-31): small touch targets outlined, and labels — low contrast, the targets' sizes, the focus
 * order — kept apart so none covers another, and left out below `A11Y_TEXT_ZOOM`, where they would cover the screens;
 * a target whose label gives way keeps its outline (#219).
 */
function A11yView({ view, nodes }: { view: View; nodes: OverlayNodes }): ReactNode {
  const a = nodes.a11y;
  if (a === undefined) {
    return null;
  }
  const outlines: ReactNode[] = [];
  const labels = findingLabels(view, nodes, a, outlines);
  for (const [id, n] of a.order) {
    const b = nodes.boxOf(id);
    if (b !== undefined) {
      const p = onScreen(b.l, b.t, view);
      labels.push({
        rect: badgeAt(p.x, p.y),
        mark: { key: `o${id}`, className: "pc-a11y-order", text: String(n), style: point(b.l, b.t, view) },
      });
    }
  }
  const kept =
    view.zoom < A11Y_TEXT_ZOOM ? [] : keepApart(labels, { width: window.innerWidth, height: window.innerHeight });
  return [
    ...outlines,
    ...kept.map(({ mark: m }) => (
      <span key={m.key} className={m.className} data-kind={m.kind} style={m.style}>
        {m.text}
      </span>
    )),
  ];
}

/** The Localisation view (PC-32): missing, untranslated and overflowing texts tagged and outlined. */
function L10nView({ view, nodes }: { view: View; nodes: OverlayNodes }): ReactNode {
  const l = nodes.l10n;
  if (l === undefined) {
    return null;
  }
  return [...l.marks].flatMap(([id, mark]) => {
    const b = nodes.boxOf(id);
    return b === undefined
      ? []
      : [
          <div key={`o${id}`} className="pc-outline" data-kind={`l10n-${mark}`} style={place(b, view)} />,
          <span key={`t${id}`} className="pc-l10n-tag" data-kind={mark} style={point(b.l, b.t, view)}>
            {l.text(mark)}
          </span>,
        ];
  });
}

/** The Size classes view (PC-33): each screen's size class, and the widgets that change with it or cross a fold. */
function SizesView({ view, nodes }: { view: View; nodes: OverlayNodes }): ReactNode {
  const z = nodes.sizes;
  if (z === undefined) {
    return null;
  }
  const screens = z.screens.flatMap(({ root, label }) => {
    const b = nodes.boxOf(root);
    return b === undefined
      ? []
      : [
          <span key={`s${root}`} className="pc-size-screen" style={point((b.l + b.r) / 2, b.t, view)}>
            {label}
          </span>,
        ];
  });
  // A tag whose words already stand at the same corner (a rail and its divider) is drawn once.
  const placed: { readonly text: string; readonly x: number; readonly y: number }[] = [];
  const fresh = (text: string, x: number, y: number) => {
    const near = placed.some((t) => t.text === text && Math.abs(t.x - x) < 120 && Math.abs(t.y - y) < 8);
    placed.push({ text, x, y });
    return !near;
  };
  const marks = [...z.marks].flatMap(([id, mark]) => {
    const b = nodes.boxOf(id);
    if (b === undefined) {
      return [];
    }
    const text = z.text(mark);
    return [
      <div key={`o${id}`} className="pc-outline" data-kind={`size-${mark.kind}`} style={place(b, view)} />,
      ...(fresh(text, b.l, b.b)
        ? [
            <span key={`t${id}`} className="pc-size-tag" data-kind={mark.kind} style={point(b.l, b.b, view)}>
              {text}
            </span>,
          ]
        : []),
    ];
  });
  return [...screens, ...marks];
}

/** The widgets the selected one flies to in a hero transition (PC-29): dashed outlines on the other screens. */
function HeroLinks({ id, view, nodes }: { id: string; view: View; nodes: OverlayNodes }): ReactNode {
  return (nodes.heroes?.(id) ?? []).flatMap((other) => {
    const b = nodes.boxOf(other);
    return b === undefined ? [] : [<div key={other} className="pc-outline" data-kind="hero" style={place(b, view)} />];
  });
}

/** The selected widget's problems under it, the most severe first, each with its code (PC-16, PC-34). */
function ProblemTag({ id, nodes }: { id: string; nodes: OverlayNodes }): ReactNode {
  const list = nodes.problems?.get(id) ?? [];
  const first = list[0];
  return first === undefined ? null : (
    <span className="pc-problem-tag" data-severity={first.severity}>
      {list.slice(0, MAX_BINDINGS).map((p) => (
        <span key={`${p.code}:${p.message}`} className="pc-problem-line" data-severity={p.severity}>
          <span className="pc-problem-message">{p.message}</span>
          <span className="pc-problem-code">{p.code}</span>
        </span>
      ))}
      {list.length > MAX_BINDINGS && <span className="pc-bind-more">+{list.length - MAX_BINDINGS}</span>}
    </span>
  );
}

/** A diamond on every component instance's corner: violet, amber when a newer version exists, red when unknown (PC-37). */
function InstanceBadges({
  view,
  nodes,
  focus,
}: {
  view: View;
  nodes: OverlayNodes;
  focus: ReadonlySet<string>;
}): ReactNode {
  const z = nodes.instances;
  if (z === undefined) {
    return null;
  }
  // A missing or outdated component always shows; a current one with every widget's badges or under the pointer (#216).
  return [...z.marks].flatMap(([id, mark]) => {
    if (nodes.allBadges !== true && !focus.has(id) && mark.known && mark.latest <= mark.version) {
      return [];
    }
    const b = nodes.boxOf(id);
    const state = !mark.known ? "missing" : mark.latest > mark.version ? "stale" : "current";
    return b === undefined
      ? []
      : [<span key={id} className="pc-instance-badge" data-state={state} style={point(b.l, b.t, view)} />];
  });
}

/** Isolation (PC-37): everything but the isolated instance dimmed. */
function Isolation({ view, nodes }: { view: View; nodes: OverlayNodes }): ReactNode {
  const id = nodes.instances?.isolated;
  const b = id === undefined ? undefined : nodes.boxOf(id);
  return b === undefined ? null : <div className="pc-isolate" style={place(b, view)} />;
}

/** The selected instance's component, version and overrides. */
function InstanceTag({ id, nodes }: { id: string; nodes: OverlayNodes }): ReactNode {
  const mark = nodes.instances?.marks.get(id);
  return mark === undefined ? null : <span className="pc-instance-tag">{nodes.instances?.tag(mark)}</span>;
}

/** Whether box `inner`'s centre lies in box `outer`. */
const centreIn = (inner: Box, outer: Box): boolean => {
  const x = (inner.l + inner.r) / 2;
  const y = (inner.t + inner.b) / 2;
  return x >= outer.l && x <= outer.r && y >= outer.t && y <= outer.b;
};

/**
 * Whether a screen's cost line shows (#229): at once when it says something — near or over the budget, problems, or
 * the screen is worked on — after a short delay when the pointer rests on the screen, else not at all.
 */
export function costShown(
  cost: ScreenCost,
  words: CostWords,
  screen: Box,
  hover: Box | undefined,
  selected: readonly { readonly box: Box }[],
): "shown" | "delayed" | "hidden" {
  const telling =
    cost.status !== "within" ||
    words.errors !== undefined ||
    words.warnings !== undefined ||
    words.timelines !== undefined ||
    selected.some((s) => centreIn(s.box, screen));
  if (telling) {
    return "shown";
  }
  return hover !== undefined && centreIn(hover, screen) ? "delayed" : "hidden";
}

/**
 * Under a screen's end corner, what it costs: its widgets against the page budget, and its problems (PC-34). Shown
 * only where it says something (#229): under a screen near or over its budget or with problems, under the screen
 * worked on, and under the one the pointer rests on, after a short delay (`data-delayed`); on every screen at once it
 * cluttered the canvas and cost a label per screen on every pan.
 */
function ScreenCosts(props: {
  readonly view: View;
  readonly nodes: OverlayNodes;
  readonly hover: Box | undefined;
  readonly selected: readonly { readonly box: Box }[];
}): ReactNode {
  const { view, nodes } = props;
  const c = nodes.costs;
  if (c === undefined) {
    return null;
  }
  return [...c.byRoot].flatMap(([root, cost]) => {
    const b = nodes.boxOf(root);
    if (b === undefined) {
      return [];
    }
    const words = c.text(cost);
    const shown = costShown(cost, words, b, props.hover, props.selected);
    if (shown === "hidden") {
      return [];
    }
    // Below the screen's overflow band, when its content runs past the frame, so it never covers that content.
    const foot = Math.max(b.b, nodes.overflowBottom?.(root) ?? b.b);
    return [
      <span
        key={root}
        className="pc-cost"
        data-status={cost.status}
        data-delayed={shown === "delayed" || undefined}
        style={point(b.r, foot, view)}
      >
        <span className="pc-cost-nodes">{words.nodes}</span>
        {words.timelines !== undefined && (
          <span className="pc-cost-part" data-kind="warning">
            {words.timelines}
          </span>
        )}
        {words.errors !== undefined && (
          <span className="pc-cost-part" data-kind="error">
            {words.errors}
          </span>
        )}
        {words.warnings !== undefined && (
          <span className="pc-cost-part" data-kind="warning">
            {words.warnings}
          </span>
        )}
      </span>,
    ];
  });
}

/** The box of the node the pointer is over, if any. */
function hoverOf(id: string | null, nodes: OverlayNodes): Box | undefined {
  return id === null ? undefined : nodes.boxOf(id);
}

function Selected(props: {
  readonly id: string;
  readonly box: Box;
  readonly view: View;
  readonly nodes: OverlayNodes;
  readonly single: boolean;
}): ReactNode {
  const { id, box, view, nodes, single } = props;
  const screen = nodes.isScreen(id);
  return (
    <>
      <div
        className="pc-outline"
        data-kind={screen ? "screen" : "selected"}
        style={{ ...place(box, view), "--r": screen ? frameCorner(nodes, view) : 0 } as CSSProperties}
      />
      {single && !screen && <Tag box={box} view={view} text={nodes.nameOf(id)} kind="selected" />}
      {single && !screen && <Spacing id={id} box={box} view={view} nodes={nodes} />}
      {single && screen && (
        // A screen's size sits under its start corner, clear of what it costs under its end (PC-34).
        <span className="pc-size" data-at="start" style={point(box.l, box.b, view)}>
          {sizeText(box)}
        </span>
      )}
      {single && !screen && (
        // One stack under the widget (C4): its size, problems, event steps, bindings and component, each below the
        // last. Placed at the corners on their own, they covered each other whenever the widget was narrow.
        <span className="pc-details" style={point((box.l + box.r) / 2, box.b, view)}>
          <span className="pc-size">{sizeText(box)}</span>
          <ProblemTag id={id} nodes={nodes} />
          <EventTag id={id} nodes={nodes} />
          <BindingTag id={id} nodes={nodes} />
          <InstanceTag id={id} nodes={nodes} />
        </span>
      )}
      {single && !screen && <HeroLinks id={id} view={view} nodes={nodes} />}
      {single &&
        !screen &&
        nodes.resizable(id) &&
        HANDLES.map((h) => {
          const at = handlePoint(box, h);
          return <span key={h} className="pc-handle" data-handle={h} style={point(at.x, at.y, view)} />;
        })}
    </>
  );
}

function Hover({
  id,
  box,
  view,
  nodes,
  tagAt,
}: {
  id: string;
  box: Box;
  view: View;
  nodes: OverlayNodes;
  /** Where the selected widget's name tag is, in canvas pixels: the hover's steps aside from it. */
  tagAt: { readonly x: number; readonly y: number } | undefined;
}): ReactNode {
  const screen = nodes.isScreen(id);
  const clear = tagClear({ x: (box.l - view.x) * view.zoom, y: (box.t - view.y) * view.zoom }, tagAt);
  return (
    <>
      <div
        className="pc-outline"
        data-kind={screen ? "screen-hover" : "hover"}
        style={{ ...place(box, view), "--r": screen ? frameCorner(nodes, view) : 0 } as CSSProperties}
      />
      {!screen && clear && <Tag box={box} view={view} text={nodes.nameOf(id)} kind="hover" />}
    </>
  );
}

function GuideMarks({ guides, view }: { guides: Guides; view: View }): ReactNode {
  const i = guides.indicator;
  return (
    <>
      {guides.lines.map((l) => {
        const box =
          l.axis === "x" ? { l: l.at, t: l.from, r: l.at, b: l.to } : { l: l.from, t: l.at, r: l.to, b: l.at };
        return (
          <div key={`${l.axis}:${l.at}:${l.from}`} className="pc-guide" data-axis={l.axis} style={place(box, view)} />
        );
      })}
      {guides.labels.map((l) => (
        <span key={`${l.x}:${l.y}:${l.text}`} className="pc-distance" style={point(l.x, l.y, view)}>
          {l.text}
        </span>
      ))}
      {i !== undefined && (
        <div
          className="pc-indicator"
          data-shape={i.r - i.l < 1 ? "column" : i.b - i.t < 1 ? "row" : "box"}
          style={place(i, view)}
        />
      )}
    </>
  );
}

/**
 * Redraws the overlay whenever the view moves, whatever moved it — the pointer, the wheel, the zoom buttons, a panel
 * revealing a node, the URL — by comparing the view once a frame. Without it a pan made outside the canvas left the
 * marks where the view had been.
 */
function useFollowView(store: OverlayStore, surface: () => CanvasSurface | null): void {
  const latest = useRef(surface);
  latest.current = surface;
  useEffect(() => {
    if (typeof requestAnimationFrame === "undefined") {
      return undefined;
    }
    let last = "";
    let frame = 0;
    const tick = () => {
      const v = latest.current()?.viewport;
      const now = v === undefined ? "" : `${v.x}:${v.y}:${v.zoom}`;
      if (now !== last) {
        last = now;
        store.viewChanged();
      }
      frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [store]);
}

/** Where the single selected widget's name tag is, and the widgets the pointer and the selection are on. */
function selectionMarks(
  selected: readonly { readonly id: string; readonly box: Box }[],
  hover: string | null,
  nodes: OverlayNodes,
  view: View,
): { tagAt: { x: number; y: number } | undefined; focus: ReadonlySet<string> } {
  const only = selected.length === 1 && !nodes.isScreen(selected[0]?.id ?? "") ? selected[0]?.box : undefined;
  return {
    tagAt: only === undefined ? undefined : { x: (only.l - view.x) * view.zoom, y: (only.t - view.y) * view.zoom },
    focus: new Set([...selected.map((s) => s.id), ...(hover === null ? [] : [hover])]),
  };
}

/** How long a refused drop's notice stays, in milliseconds. */
const NOTICE_MS = 2800;

/** The text of a refused drop. */
function refusedText(nodes: OverlayNodes, r: RefusedDrop): string {
  return nodes.dropText?.refused(r, r.at === null ? "" : nodes.nameOf(r.at)) ?? r.reason;
}

/** The container a drop lands in, with its slot on the tag; or, refused, the widget it is about and why, in red. */
function DropMarks({ state, view, nodes }: { state: OverlayState; view: View; nodes: OverlayNodes }): ReactNode {
  const dropBox = state.drop === null ? undefined : nodes.boxOf(state.drop);
  const refused = state.refused;
  const refusedBox = refused?.at == null ? undefined : nodes.boxOf(refused.at);
  return (
    <>
      {dropBox !== undefined && state.drop !== null && (
        <>
          <div className="pc-outline" data-kind="drop" style={place(dropBox, view)} />
          <Tag
            box={dropBox}
            view={view}
            text={
              state.dropSlot === null ? nodes.nameOf(state.drop) : `${nodes.nameOf(state.drop)} › ${state.dropSlot}`
            }
            kind="drop"
          />
        </>
      )}
      {refused !== null && refusedBox !== undefined && (
        <>
          <div className="pc-outline" data-kind="refused" style={place(refusedBox, view)} />
          <Tag box={refusedBox} view={view} text={refusedText(nodes, refused)} kind="refused" />
        </>
      )}
    </>
  );
}

/**
 * The words a refused drop leaves where it was let go, for a moment, in a polite live region: the overlay is hidden
 * from assistive technology, the region is not.
 */
function DropNotice({ store, view, nodes }: { store: OverlayStore; view: View; nodes: OverlayNodes }): ReactNode {
  const notice = useOverlayState(store).notice;
  useEffect(() => {
    if (notice === null) {
      return undefined;
    }
    const timer = setTimeout(() => store.set({ notice: null }), NOTICE_MS);
    return () => clearTimeout(timer);
  }, [notice, store]);
  const text =
    notice === null ? "" : (nodes.dropText?.notice(refusedText(nodes, notice)) ?? refusedText(nodes, notice));
  return (
    <div role="status" className="pc-notice-region">
      {notice !== null && (
        <span className="pc-notice" style={point(notice.x, notice.y, view)}>
          {text}
        </span>
      )}
    </div>
  );
}

/**
 * The interaction overlay of the plugin canvas (STU-067), in the page's DOM above the canvas and below the docks: the
 * selection with its tag, size and handles (a screen as its device frame), the node under the pointer, the container
 * the selection is inside, the marquee, the box of a drag or resize, snap guides with distances, the insertion
 * indicator and the container a drop lands in. It follows the view on every pan and zoom; the worker draws nothing of
 * it, so the screens never re-render for the pointer.
 */
export function InteractionOverlay(props: {
  readonly store: OverlayStore;
  readonly surface: () => CanvasSurface | null;
  readonly selection: Selection;
  readonly nodes: OverlayNodes;
}): ReactNode {
  const state = useOverlayState(props.store);
  useFollowView(props.store, props.surface);
  const surface = props.surface();
  if (surface === null) {
    return null;
  }
  const v = surface.viewport;
  const view = { x: v.x, y: v.y, zoom: v.zoom };
  const { nodes, selection } = props;
  const selected = selection.ids.flatMap((id) => {
    const box = nodes.boxOf(id);
    return box === undefined ? [] : [{ id, box }];
  });
  const scopeBox = nodes.scope === null ? undefined : nodes.boxOf(nodes.scope);
  const hoverBox = state.hover === null || selection.ids.includes(state.hover) ? undefined : nodes.boxOf(state.hover);
  const busy = state.ghost !== null || state.marquee !== null;
  const { tagAt, focus } = selectionMarks(selected, state.hover, nodes, view);
  return (
    <>
      <div className="pc-overlay" aria-hidden="true" data-busy={busy || undefined}>
        {scopeBox !== undefined && !nodes.isScreen(nodes.scope ?? "") && (
          <div className="pc-outline" data-kind="scope" style={place(scopeBox, view)} />
        )}
        {hoverBox !== undefined && state.hover !== null && !busy && (
          <Hover id={state.hover} box={hoverBox} view={view} nodes={nodes} tagAt={tagAt} />
        )}
        <DropMarks state={state} view={view} nodes={nodes} />
        {selected.map((s) => (
          <Selected
            key={s.id}
            id={s.id}
            box={s.box}
            view={view}
            nodes={nodes}
            single={selected.length === 1 && state.ghost === null}
          />
        ))}
        {state.ghost !== null && (
          <>
            <div className="pc-outline" data-kind="ghost" style={place(state.ghost, view)} />
            <span className="pc-size" style={point((state.ghost.l + state.ghost.r) / 2, state.ghost.b, view)}>
              {sizeText(state.ghost)}
            </span>
          </>
        )}
        <A11yView view={view} nodes={nodes} />
        <L10nView view={view} nodes={nodes} />
        <SizesView view={view} nodes={nodes} />
        <ScreenCosts view={view} nodes={nodes} hover={hoverOf(state.hover, nodes)} selected={selected} />
        <InstanceBadges view={view} nodes={nodes} focus={focus} />
        <Isolation view={view} nodes={nodes} />
        <CornerBadges view={view} nodes={nodes} focus={focus} />
        {state.guides !== null && <GuideMarks guides={state.guides} view={view} />}
        {state.marquee !== null && (
          <div
            className="pc-marquee"
            style={
              {
                "--x": state.marquee.l,
                "--y": state.marquee.t,
                "--w": state.marquee.r - state.marquee.l,
                "--h": state.marquee.b - state.marquee.t,
              } as CSSProperties
            }
          />
        )}
      </div>
      <DropNotice store={props.store} view={view} nodes={nodes} />
    </>
  );
}
