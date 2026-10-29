// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { PluginSummary } from "@plux/api";
import type { Bundle, NavigateEdge } from "@plux/canvas";
import { Button, Dialog, Field, Input, StatusPill } from "@plux/ui";
import { type ReactNode, useEffect, useState } from "react";
import { type ArrowKey, nudgeDelta } from "../plugin-canvas/select/geometry.ts";
import type { appCanvasMessages } from "./app-canvas.messages.ts";
import type { Frame, Point } from "./app-canvas.model.ts";
import type { Badge, SceneBox } from "./app-canvas.scene.ts";
import type { MiniMap } from "./plugin-minimap.ts";

type Messages = ReturnType<typeof appCanvasMessages>;

const ARROWS: ReadonlySet<string> = new Set(["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"]);

interface CardProps {
  readonly box: SceneBox;
  readonly selected: boolean;
  readonly messages: Messages;
  readonly onPointerDown: (e: React.PointerEvent) => void;
  readonly onOpen: () => void;
  readonly onHover: (on: boolean) => void;
  /** Keyboard selection (Space, or a click from assistive technology); `additive` with Shift. */
  readonly onSelect?: (additive: boolean) => void;
  /** Arrow keys move the selection by a nudge (1, with Shift 10 canvas units). */
  readonly onNudge?: (delta: Point) => void;
}

/**
 * One box in the accessible layer of the app canvas (STU-011, ADR-0013 as revised): the canvas draws
 * the box, this button names it with every badge the drawing shows, takes keyboard focus and offers
 * the same actions as the pointer: select, open, nudge.
 */
export function PluginBoxCard(props: CardProps): ReactNode {
  const { box, messages: m } = props;
  const name = box.title;
  return (
    <button
      type="button"
      aria-label={box.native ? `${m.native} ${box.key}` : m.plugin(name, box.key)}
      aria-pressed={props.selected}
      data-plugin={box.key}
      className="flex flex-col items-start gap-1 text-start"
      onPointerDown={props.onPointerDown}
      onDoubleClick={props.onOpen}
      onClick={(e) => {
        if (e.detail === 0) {
          props.onSelect?.(e.shiftKey);
        }
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          props.onOpen();
        } else if (e.key === " ") {
          e.preventDefault();
          props.onSelect?.(e.shiftKey);
        } else if (ARROWS.has(e.key) && props.onNudge !== undefined) {
          e.preventDefault();
          props.onNudge(nudgeDelta(e.key as ArrowKey, e.shiftKey));
        }
      }}
      onFocus={() => props.onHover(true)}
      onBlur={() => props.onHover(false)}
    >
      <span className="text-title">{name}</span>
      <span className="text-data text-secondary">{box.key}</span>
      <span className="flex flex-wrap gap-1">
        {box.badges.map((b) => (
          <StatusPill key={b.label} status={b.status} label={b.label} />
        ))}
      </span>
    </button>
  );
}

/** Who published a plugin's newest version, and when (STU-031). */
export interface LastPublished {
  readonly by: string;
  readonly at: string;
}

/** The mini-map of a plugin's screens: a small box per screen and a line per navigation between them. */
function MiniMapView(props: { map: MiniMap; messages: Messages }): ReactNode {
  const { map, messages: m } = props;
  if (map.screens.length === 0) {
    return <span className="text-data text-secondary">{m.noScreens}</span>;
  }
  const centre = new Map(
    map.screens.map((s) => [s.id, { x: s.x + map.cell.width / 2, y: s.y + map.cell.height / 2 }] as const),
  );
  return (
    <svg
      role="img"
      aria-label={m.miniMap(map.screens.map((s) => s.title).join(", "))}
      viewBox={`0 0 ${map.width} ${map.height}`}
      className="w-full rounded-sm border bg-canvas"
    >
      {map.edges.map((e) => {
        const from = centre.get(e.from);
        const to = centre.get(e.to);
        return from === undefined || to === undefined ? null : (
          <line
            key={`${e.from}>${e.to}`}
            x1={from.x}
            y1={from.y}
            x2={to.x}
            y2={to.y}
            className="stroke-current text-secondary"
          />
        );
      })}
      {map.screens.map((s) => (
        <rect
          key={s.id}
          data-screen={s.id}
          x={s.x}
          y={s.y}
          width={map.cell.width}
          height={map.cell.height}
          rx={2}
          className="fill-current text-accent"
        />
      ))}
    </svg>
  );
}

/**
 * The hover/focus detail card of a plugin (E5, STU-031): description, a mini-map of its screens, link counts in and
 * out, the functions it uses, its bundle size, and who published it last and when. Floating chrome over the canvas.
 */
export function DetailCard(props: {
  summary: PluginSummary;
  badges: readonly Badge[];
  messages: Messages;
  miniMap: MiniMap;
  published: LastPublished | undefined;
}): ReactNode {
  const { summary: s, messages: m } = props;
  const functions = [...s.functionsUsed];
  return (
    <aside
      aria-label={s.plugin?.name}
      className="pointer-events-none absolute end-4 top-4 flex w-64 flex-col gap-1 rounded-md border bg-raised p-3"
    >
      <span className="text-section">{s.plugin?.name}</span>
      <span className="flex flex-wrap gap-1">
        {props.badges.map((b) => (
          <StatusPill key={b.label} status={b.status} label={b.label} />
        ))}
      </span>
      {s.plugin?.description ? <span>{s.plugin.description}</span> : null}
      <MiniMapView map={props.miniMap} messages={m} />
      <span className="text-data">{m.screens(Number(s.screens))}</span>
      <span className="text-data">{m.versions(Number(s.versions))}</span>
      <span className="text-data">{m.bundleSize(Math.ceil(Number(s.bundleSize) / 1024))}</span>
      <span className="text-data">{m.linksOut(s.linksOut.length)}</span>
      <span className="text-data">{m.linksIn(s.linksIn.length)}</span>
      <span className="text-data">{m.functions(functions.length)}</span>
      {functions.length > 0 ? (
        <span className="text-data text-secondary">{m.functionsUsed(functions.join(", "))}</span>
      ) : null}
      <span className="text-data">
        {props.published === undefined
          ? m.neverPublished
          : props.published.by === ""
            ? m.publishedAt(props.published.at)
            : m.publishedBy(props.published.by, props.published.at)}
      </span>
    </aside>
  );
}

/** "Move to…": places a box at exact coordinates, the keyboard alternative to dragging (WCAG 2.5.7). */
export function MoveToDialog(props: {
  readonly box: SceneBox | undefined;
  readonly messages: Messages;
  readonly onMove: (to: Point) => void;
  readonly onClose: () => void;
}): ReactNode {
  const { box, messages: m } = props;
  const [x, setX] = useState("");
  const [y, setY] = useState("");
  useEffect(() => {
    setX(String(Math.round(box?.x ?? 0)));
    setY(String(Math.round(box?.y ?? 0)));
  }, [box]);
  if (box === undefined) {
    return null;
  }
  const submit = () => {
    const to = { x: Number(x), y: Number(y) };
    if (Number.isFinite(to.x) && Number.isFinite(to.y)) {
      props.onMove(to);
    }
  };
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && props.onClose()}
      title={m.moveTo(box.title)}
      size="sm"
      labels={{ close: m.close }}
    >
      <Field label={m.moveX}>
        <Input inputMode="numeric" value={x} onChange={(e) => setX(e.target.value)} />
      </Field>
      <Field label={m.moveY}>
        <Input inputMode="numeric" value={y} onChange={(e) => setY(e.target.value)} />
      </Field>
      <div className="flex justify-end gap-2">
        <Button onClick={props.onClose}>{m.cancel}</Button>
        <Button onClick={submit}>{m.save}</Button>
      </div>
    </Dialog>
  );
}

export interface BoxListProps {
  readonly boxes: readonly SceneBox[];
  readonly bundles: readonly Bundle[];
  readonly frames: readonly Frame[];
  readonly selected: ReadonlySet<string>;
  readonly messages: Messages;
  readonly onPointerDown: (e: React.PointerEvent, box: SceneBox) => void;
  readonly onSelect: (box: SceneBox, additive: boolean) => void;
  readonly onOpen: (box: SceneBox) => void;
  readonly onFocusBox: (id: string | undefined) => void;
  readonly onNudge: (box: SceneBox, delta: Point) => void;
  readonly onFocusBundle: (id: string | null) => void;
  readonly onLane: (link: NavigateEdge) => void;
}

/**
 * The accessible layer of the app canvas: every box and every arrow with its lanes as focusable
 * elements, visually hidden because the canvas draws them (the focused box gets a drawn focus ring).
 */
export function BoxList(props: BoxListProps): ReactNode {
  const { boxes, bundles, messages: m } = props;
  const titleOf = new Map(boxes.map((b) => [b.id, b.title]));
  return (
    <div className="sr-only">
      <ul aria-label={m.boxes}>
        {boxes.map((b) => (
          <li key={b.id}>
            <PluginBoxCard
              box={b}
              selected={props.selected.has(b.id)}
              messages={m}
              onPointerDown={(e) => props.onPointerDown(e, b)}
              onOpen={() => props.onOpen(b)}
              onHover={(on) => props.onFocusBox(on ? b.id : undefined)}
              onSelect={(additive) => props.onSelect(b, additive)}
              onNudge={(d) => props.onNudge(b, d)}
            />
          </li>
        ))}
      </ul>
      <ul>
        {props.frames.map((f) => (
          <li key={f.id}>{m.frame(f.label)}</li>
        ))}
      </ul>
      <ul aria-label={m.links}>
        {bundles.map((bundle) => (
          <li key={bundle.id}>
            <span>
              {m.bundle(
                titleOf.get(bundle.source) ?? bundle.source,
                titleOf.get(bundle.target) ?? bundle.target,
                bundle.links.length,
              )}
            </span>
            <ul>
              {bundle.links.map((link) => (
                <li key={link.id}>
                  <button
                    type="button"
                    onFocus={() => props.onFocusBundle(bundle.id)}
                    onBlur={() => props.onFocusBundle(null)}
                    onClick={() => props.onLane(link)}
                  >
                    {m.laneMenu(link.title)}
                  </button>
                </li>
              ))}
            </ul>
          </li>
        ))}
      </ul>
    </div>
  );
}
