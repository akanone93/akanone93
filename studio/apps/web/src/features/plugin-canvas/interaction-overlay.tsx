// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Guides } from "@plux/canvas";
import type { CSSProperties, ReactNode } from "react";
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

/** The corner radius of a device frame, in world units, as the canvas draws it. */
const FRAME_RADIUS = 32;

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
        style={{ ...place(box, view), "--r": screen ? FRAME_RADIUS * view.zoom : 0 } as CSSProperties}
      />
      {screen && <ScreenName box={box} view={view} text={nodes.nameOf(id)} kind="selected" />}
      {single && !screen && <Tag box={box} view={view} text={nodes.nameOf(id)} kind="selected" />}
      {single && (
        <span className="pc-size" style={point((box.l + box.r) / 2, box.b, view)}>
          {sizeText(box)}
        </span>
      )}
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
        style={{ ...place(box, view), "--r": screen ? FRAME_RADIUS * view.zoom : 0 } as CSSProperties}
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
