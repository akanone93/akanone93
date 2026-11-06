// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Guides } from "@plux/canvas";
import { type CSSProperties, type ReactNode, useEffect, useRef } from "react";
import type { NodeProblem } from "../problems/problem-nodes.ts";
import { type LowContrast, smallTarget } from "./a11y-marks.ts";
import type { BindingMark } from "./binding-marks.ts";
import type { L10nOverlay } from "./l10n-marks.ts";
import type { HandlerStep } from "./quick-actions.ts";
import type { SizesOverlay } from "./responsive.ts";
import type { CostsOverlay } from "./screen-costs.ts";
import type { Box } from "./select/geometry.ts";
import { HANDLES, handlePoint } from "./select/handles.ts";
import { type OverlayStore, useOverlayState } from "./select/overlay-store.ts";
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
  return (
    <span className="pc-tag" data-kind={kind} style={point(box.l, box.t, view)}>
      {text}
    </span>
  );
}

/** The corner radius of a device frame without chrome, in world units, as the canvas draws it. */
const FRAME_RADIUS = 32;

/** A screen's outline radius at the view's zoom: its device's corners. */
const frameCorner = (nodes: OverlayNodes, view: View): number => (nodes.frameRadius ?? FRAME_RADIUS) * view.zoom;

/** The world offset of a screen's name above its frame, as the canvas draws it (`titleId`: 30 above, 15 high). */
const TITLE_ABOVE = 32;

/**
 * A screen's name above its frame, marked: the canvas draws the name, this chip covers it in the selection's colour at
 * the same place and size, as design tools mark a selected frame's name.
 */
function ScreenName({ box, view, text, kind }: { box: Box; view: View; text: string; kind: string }): ReactNode {
  const style = { ...point(box.l, box.t - TITLE_ABOVE, view), "--z": view.zoom } as CSSProperties;
  return (
    <span className="pc-title" data-kind={kind} style={style}>
      {text}
    </span>
  );
}

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

/** A dot on every widget with a problem, red for an error and amber for a warning (PC-16). */
function ProblemDots({ view, nodes }: { view: View; nodes: OverlayNodes }): ReactNode {
  return [...(nodes.problems ?? new Map<string, readonly NodeProblem[]>())].flatMap(([id, list]) => {
    const box = nodes.boxOf(id);
    const first = list[0];
    return box === undefined || first === undefined || first.severity === "info"
      ? []
      : [<span key={id} className="pc-problem-dot" data-severity={first.severity} style={point(box.r, box.t, view)} />];
  });
}

/** A badge on the top-end corner of every bound widget (PC-23); its count when it binds several props. */
function BindingBadges({ view, nodes }: { view: View; nodes: OverlayNodes }): ReactNode {
  return [...(nodes.bindings ?? new Map<string, readonly BindingMark[]>())].flatMap(([id, list]) => {
    const box = nodes.boxOf(id);
    return box === undefined || nodes.isScreen(id)
      ? []
      : [
          <span
            key={id}
            className="pc-bind-badge"
            style={point(box.r, box.t, view)}
            title={list.map((b) => b.expr).join("\n")}
          >
            {list.length > 1 ? list.length : ""}
          </span>,
        ];
  });
}

const MAX_BINDINGS = 3;

/** An amber badge beside the binding badge of every widget whose events run steps (PC-27). */
function EventBadges({ view, nodes }: { view: View; nodes: OverlayNodes }): ReactNode {
  return [...(nodes.events ?? new Map<string, readonly HandlerStep[]>())].flatMap(([id, steps]) => {
    const box = nodes.boxOf(id);
    return box === undefined || nodes.isScreen(id)
      ? []
      : [
          <span key={id} className="pc-event-badge" style={point(box.r, box.t, view)}>
            {steps.length > 1 ? steps.length : ""}
          </span>,
        ];
  });
}

/** The selected widget's event handlers under its start corner: each event and the steps it runs. */
function EventTag({ id, box, view, nodes }: { id: string; box: Box; view: View; nodes: OverlayNodes }): ReactNode {
  const steps = nodes.events?.get(id) ?? [];
  if (steps.length === 0) {
    return null;
  }
  return (
    <span className="pc-event-tag" style={point(box.l, box.b, view)}>
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

/** The selected widget's bindings over its top-end corner: each prop, its expression and the value drawn. */
function BindingTag({ id, box, view, nodes }: { id: string; box: Box; view: View; nodes: OverlayNodes }): ReactNode {
  const list = nodes.bindings?.get(id) ?? [];
  if (list.length === 0) {
    return null;
  }
  return (
    <span className="pc-bind-tag" style={point(box.r, box.b, view)}>
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

/** The Accessibility view (PC-31): focus order numbers, small touch targets outlined, low-contrast texts badged. */
function A11yView({ view, nodes }: { view: View; nodes: OverlayNodes }): ReactNode {
  const a = nodes.a11y;
  if (a === undefined) {
    return null;
  }
  const order = [...a.order].flatMap(([id, n]) => {
    const b = nodes.boxOf(id);
    return b === undefined
      ? []
      : [
          <span key={`o${id}`} className="pc-a11y-order" style={point(b.l, b.t, view)}>
            {n}
          </span>,
        ];
  });
  const small = [...a.targets].flatMap((id) => {
    const b = nodes.boxOf(id);
    const size = b === undefined ? undefined : { w: b.r - b.l, h: b.b - b.t };
    return b === undefined || size === undefined || !smallTarget(size, a.min)
      ? []
      : [
          <div key={`t${id}`} className="pc-outline" data-kind="a11y-target" style={place(b, view)} />,
          <span key={`l${id}`} className="pc-a11y-tag" style={point(b.l, b.b, view)}>
            {nodes.a11yText?.target(Math.round(size.w), Math.round(size.h), a.min)}
          </span>,
        ];
  });
  const low = [...a.low].flatMap(([id, c]) => {
    const b = nodes.boxOf(id);
    return b === undefined
      ? []
      : [
          <span key={`c${id}`} className="pc-a11y-tag" data-kind="contrast" style={point(b.r, b.t, view)}>
            {nodes.a11yText?.contrast(c.ratio.toFixed(1), String(c.needed))}
          </span>,
        ];
  });
  return [...small, ...low, ...order];
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

/** The selected widget's first problem, under its size. */
/** The selected widget's problems under it, the most severe first, each with its code (PC-16, PC-34). */
function ProblemTag({ id, box, view, nodes }: { id: string; box: Box; view: View; nodes: OverlayNodes }): ReactNode {
  const list = nodes.problems?.get(id) ?? [];
  const first = list[0];
  return first === undefined ? null : (
    <span className="pc-problem-tag" data-severity={first.severity} style={point((box.l + box.r) / 2, box.b, view)}>
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

/** Under each screen's end corner, what it costs: its widgets against the page budget, and its problems (PC-34). */
function ScreenCosts({ view, nodes }: { view: View; nodes: OverlayNodes }): ReactNode {
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
    return [
      <span key={root} className="pc-cost" data-status={cost.status} title={words.title} style={point(b.r, b.b, view)}>
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
      {screen && <ScreenName box={box} view={view} text={nodes.nameOf(id)} kind="selected" />}
      {single && !screen && <Tag box={box} view={view} text={nodes.nameOf(id)} kind="selected" />}
      {single && !screen && <Spacing id={id} box={box} view={view} nodes={nodes} />}
      {single && (
        <span className="pc-size" style={point((box.l + box.r) / 2, box.b, view)}>
          {sizeText(box)}
        </span>
      )}
      {single && !screen && <ProblemTag id={id} box={box} view={view} nodes={nodes} />}
      {single && !screen && <BindingTag id={id} box={box} view={view} nodes={nodes} />}
      {single && !screen && <EventTag id={id} box={box} view={view} nodes={nodes} />}
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

function Hover({ id, box, view, nodes }: { id: string; box: Box; view: View; nodes: OverlayNodes }): ReactNode {
  const screen = nodes.isScreen(id);
  return (
    <>
      <div
        className="pc-outline"
        data-kind={screen ? "screen-hover" : "hover"}
        style={{ ...place(box, view), "--r": screen ? frameCorner(nodes, view) : 0 } as CSSProperties}
      />
      {screen ? (
        <ScreenName box={box} view={view} text={nodes.nameOf(id)} kind="hover" />
      ) : (
        <Tag box={box} view={view} text={nodes.nameOf(id)} kind="hover" />
      )}
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
  const dropBox = state.drop === null ? undefined : nodes.boxOf(state.drop);
  const busy = state.ghost !== null || state.marquee !== null;
  return (
    <div className="pc-overlay" aria-hidden="true" data-busy={busy || undefined}>
      {scopeBox !== undefined && !nodes.isScreen(nodes.scope ?? "") && (
        <div className="pc-outline" data-kind="scope" style={place(scopeBox, view)} />
      )}
      {hoverBox !== undefined && state.hover !== null && !busy && (
        <Hover id={state.hover} box={hoverBox} view={view} nodes={nodes} />
      )}
      {dropBox !== undefined && state.drop !== null && (
        <>
          <div className="pc-outline" data-kind="drop" style={place(dropBox, view)} />
          <Tag box={dropBox} view={view} text={nodes.nameOf(state.drop)} kind="drop" />
        </>
      )}
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
      <ScreenCosts view={view} nodes={nodes} />
      <BindingBadges view={view} nodes={nodes} />
      <EventBadges view={view} nodes={nodes} />
      <ProblemDots view={view} nodes={nodes} />
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
  );
}
