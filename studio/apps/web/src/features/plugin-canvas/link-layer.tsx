// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { frameId, ProvisionalArrow } from "@plux/canvas";
import { Button, Icon, Menu } from "@plux/ui";
import { useEffect, useRef, useState } from "react";
import { handleAnchor, type Point, screenAt, toCanvas, toWorld, type WorldRect } from "./link-drag.ts";
import { linkMessages } from "./links.messages.ts";
import { type ApplyMode, eventsOf, linkRecipe } from "./links.ts";
import { uuidv7 } from "./screens.ts";
import type { EditFn } from "./select/edits.ts";
import { itemIdOf } from "./select/tree.ts";
import type { CanvasSurface } from "./surface.ts";
import { TriggerPicker } from "./trigger-picker.tsx";

export interface LinkLayerProps {
  readonly surface: () => CanvasSurface | null;
  readonly canvas: () => HTMLCanvasElement | null;
  /** The single selected widget, if any. */
  readonly widget:
    | { readonly id: string; readonly screen: string; readonly node: { readonly type?: unknown } }
    | undefined;
  readonly screens: readonly { readonly id: string; readonly title: string; readonly route: string }[];
  readonly canEdit: boolean;
  readonly edit: EditFn;
  readonly newId?: () => string;
}

interface Drop {
  readonly route: string;
  readonly title: string;
}

/**
 * The link handle on the selected widget (F6, STU-047). Dragging it to a screen draws a provisional
 * arrow and, on drop, opens the trigger picker (E7); the keyboard alternative is the menu button
 * beside it, which lists the screens and opens the same picker. Nothing is written until the picker
 * is confirmed, and then the same recipe as the Actions tab runs through the edit pipeline.
 */
export function LinkLayer(props: LinkLayerProps) {
  const m = linkMessages(useLingui().i18n);
  const [at, setAt] = useState<Point | undefined>(undefined);
  const [drop, setDrop] = useState<Drop | undefined>(undefined);
  const [said, setSaid] = useState("");
  const drag = useRef<{ arrow: ProvisionalArrow } | null>(null);
  const type = typeof props.widget?.node.type === "string" ? props.widget.node.type : undefined;
  const events = eventsOf(type);
  const active = props.canEdit && props.widget !== undefined && events.length > 0;
  const widgetKey = props.widget === undefined ? undefined : itemIdOf(props.widget.screen, props.widget.id);

  // The handle follows the widget while the view or the layout changes.
  useEffect(() => {
    if (!active || widgetKey === undefined) {
      setAt(undefined);
      return;
    }
    let frame = 0;
    const measure = () => {
      const surface = props.surface();
      const box = surface?.boundsOf?.(widgetKey);
      if (surface !== null && box !== undefined) {
        const p = toCanvas(surface.viewport, handleAnchor(box));
        setAt((prev) => (prev !== undefined && prev.x === p.x && prev.y === p.y ? prev : p));
      }
      frame = requestAnimationFrame(measure);
    };
    measure();
    return () => cancelAnimationFrame(frame);
  }, [active, widgetKey, props.surface]);

  const frames = (surface: CanvasSurface): { id: string; box: WorldRect }[] =>
    props.screens.flatMap((s) => {
      const box = surface.boundsOf?.(frameId(s.id));
      return box === undefined ? [] : [{ id: s.id, box }];
    });

  const finish = (route: string | undefined) => {
    const surface = props.surface();
    const d = drag.current;
    drag.current = null;
    if (d !== null && surface?.patch !== undefined) {
      surface.patch(d.arrow.clear());
    }
    const target = props.screens.find((s) => s.route === route);
    if (target === undefined) {
      setSaid(m.cancelled);
    } else {
      setDrop({ route: target.route, title: target.title });
    }
  };

  const onPointerDown = (e: React.PointerEvent<HTMLSpanElement>) => {
    const surface = props.surface();
    if (surface === null || at === undefined || e.button !== 0) {
      return;
    }
    e.currentTarget.setPointerCapture(e.pointerId);
    drag.current = { arrow: new ProvisionalArrow() };
    e.preventDefault();
  };

  const pointerWorld = (e: React.PointerEvent, surface: CanvasSurface): Point | undefined => {
    const box = props.canvas()?.getBoundingClientRect();
    return box === undefined
      ? undefined
      : toWorld(surface.viewport, { x: e.clientX - box.left, y: e.clientY - box.top });
  };

  const onPointerMove = (e: React.PointerEvent<HTMLSpanElement>) => {
    const surface = props.surface();
    const d = drag.current;
    if (surface === null || d === null || props.widget === undefined) {
      return;
    }
    const box = surface.boundsOf?.(itemIdOf(props.widget.screen, props.widget.id));
    const to = pointerWorld(e, surface);
    if (box !== undefined && to !== undefined) {
      surface.patch?.(d.arrow.update(handleAnchor(box), to));
    }
  };

  const onPointerUp = (e: React.PointerEvent<HTMLSpanElement>) => {
    const surface = props.surface();
    if (drag.current === null || surface === null) {
      return;
    }
    const world = pointerWorld(e, surface);
    const id = world === undefined ? undefined : screenAt(world, frames(surface));
    finish(props.screens.find((s) => s.id === id)?.route);
  };

  const confirm = (event: string, mode: ApplyMode) => {
    if (props.widget === undefined || drop === undefined) {
      return;
    }
    props.edit(props.widget.screen, linkRecipe(props.widget.id, event, drop.route, (props.newId ?? uuidv7)(), mode));
    setSaid(m.created(event, drop.title));
    setDrop(undefined);
  };

  return (
    <>
      {active && at !== undefined && (
        <div className="pointer-events-none absolute inset-0">
          <div
            className="pointer-events-auto absolute flex -translate-y-1/2 items-center gap-1"
            style={{ left: at.x, top: at.y }}
          >
            <span
              aria-hidden="true"
              className="flex size-6 cursor-crosshair touch-none items-center justify-center rounded-full bg-raised shadow-e2"
              onPointerDown={onPointerDown}
              onPointerMove={onPointerMove}
              onPointerUp={onPointerUp}
              onPointerCancel={() => finish(undefined)}
            >
              <Icon name="arrow-right" size="capsule" />
            </span>
            <Menu
              trigger={
                <Button size="sm" variant="ghost" aria-label={m.handleChoose}>
                  <Icon name="dots-three" size="capsule" />
                </Button>
              }
              entries={props.screens.map((s) => ({
                kind: "item" as const,
                id: s.id,
                label: s.title,
                onSelect: () => setDrop({ route: s.route, title: s.title }),
              }))}
            />
          </div>
        </div>
      )}
      <p role="status" aria-live="polite" className="sr-only">
        {said}
      </p>
      {props.widget !== undefined && drop !== undefined && (
        <TriggerPicker
          open
          node={props.widget.node}
          events={events}
          target={drop.title}
          onConfirm={confirm}
          onCancel={() => setDrop(undefined)}
        />
      )}
    </>
  );
}
