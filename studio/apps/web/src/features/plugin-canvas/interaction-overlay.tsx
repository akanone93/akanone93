// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Guides } from "@plux/canvas";
import { type CSSProperties, type ReactNode, useEffect, useRef } from "react";
import type { NodeProblem } from "../problems/problem-nodes.ts";
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

/** The selected widget's first problem, under its size. */
function ProblemTag({ id, box, view, nodes }: { id: string; box: Box; view: View; nodes: OverlayNodes }): ReactNode {
  const first = nodes.problems?.get(id)?.[0];
  return first === undefined ? null : (
    <span className="pc-problem-tag" data-severity={first.severity} style={point((box.l + box.r) / 2, box.b, view)}>
      {first.message}
    </span>
  );
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
