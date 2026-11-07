// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { titleId } from "@plux/canvas";
import { Icon, type IconName, Menu, type MenuEntry, Tooltip } from "@plux/ui";
import {
  type ButtonHTMLAttributes,
  type CSSProperties,
  type KeyboardEvent,
  type MouseEvent,
  type PointerEvent,
  type ReactNode,
  type Ref,
  useEffect,
  useRef,
  useState,
} from "react";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import type { PageKind } from "./scene.ts";
import { screenHeaderMessages } from "./screen-header.messages.ts";
import { slug } from "./screens.ts";
import type { Box } from "./select/geometry.ts";
import { type OverlayStore, useOverlayState } from "./select/overlay-store.ts";
import type { CanvasSurface } from "./surface.ts";
import "./screen-header.css";

/** A screen as its header shows it (MR-2). */
export interface HeaderScreen {
  readonly id: string;
  /** The screen's root node: what selecting the screen selects. */
  readonly root: string;
  readonly title: string;
  readonly route?: string | undefined;
  readonly kind: PageKind;
  /** The plugin opens on this screen. */
  readonly entry: boolean;
}

/** What a header asks the page to do for its screen. */
export type HeaderAction = "settings" | "matrix" | "zoom" | "duplicate" | "entry" | "move" | "delete";

/**
 * The canvas's pointer handlers (STU-067). A header hands them its presses as a hit on its screen's title, so a press
 * selects the screen, a drag moves it with the canvas's snapping, and a secondary click opens the canvas's screen menu.
 */
export interface HeaderPointer {
  readonly down: (e: PointerEvent<HTMLElement>, hits: readonly string[]) => void;
  readonly move: (e: PointerEvent<HTMLElement>) => void;
  readonly up: (e: PointerEvent<HTMLElement>) => void;
  readonly cancel: (e: PointerEvent<HTMLElement>) => void;
  readonly menu: (e: MouseEvent<HTMLElement>, hits: readonly string[]) => void;
}

/** The icon of each page kind (SCH-022): a phone for a screen, a window for a dialog, rows for a sheet. */
const KIND_ICON: Readonly<Record<PageKind, IconName>> = {
  screen: "device-mobile",
  dialog: "frame",
  bottomSheet: "rows",
  fullscreenDialog: "maximize",
};

/** How far a press may move and still be a click on the title, in CSS pixels. */
const CLICK_PX = 4;

interface View {
  readonly x: number;
  readonly y: number;
  readonly zoom: number;
}

/** A header's place in canvas pixels: its frame's top start, and the frame's width at the view's zoom. */
export function headerPlace(box: Box, view: View): { readonly x: number; readonly y: number; readonly w: number } {
  return {
    x: (box.l - view.x) * view.zoom,
    y: (box.t - view.y) * view.zoom,
    w: Math.max(0, (box.r - box.l) * view.zoom),
  };
}

/** Whether the route name says more than the title: shown only when it is not the title's own slug. */
export function routeWorthShowing(title: string, route: string | undefined): boolean {
  return route !== undefined && route !== "" && route !== slug(title);
}

/** The field that renames a screen in place: Enter or leaving it keeps the new title, Escape keeps the old one. */
function TitleField(props: {
  readonly title: string;
  readonly label: string;
  readonly onDone: (title: string | undefined) => void;
}): ReactNode {
  const ref = useRef<HTMLInputElement>(null);
  const done = useRef(false);
  const finish = (value: string | undefined) => {
    if (!done.current) {
      done.current = true;
      props.onDone(value);
    }
  };
  useEffect(() => {
    // After a menu closes it hands focus back to its button: the field takes it on the next frame.
    const frame = requestAnimationFrame(() => {
      ref.current?.focus();
      ref.current?.select();
    });
    return () => cancelAnimationFrame(frame);
  }, []);
  const key = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      finish(e.currentTarget.value);
    } else if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      finish(undefined);
    }
  };
  return (
    <input
      ref={ref}
      className="pc-header-field"
      aria-label={props.label}
      defaultValue={props.title}
      data-no-grip=""
      onKeyDown={key}
      onBlur={(e) => finish(e.currentTarget.value)}
    />
  );
}

/** An icon button of the header; it never grips the screen. A menu hands its trigger props and ref through `rest`. */
function HeaderButton({
  icon,
  label,
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  readonly icon: IconName;
  readonly label: string;
  readonly ref?: Ref<HTMLButtonElement>;
}): ReactNode {
  return (
    <button {...rest} type="button" className="pc-header-button" aria-label={label} data-no-grip="">
      <Icon name={icon} size="pill" />
    </button>
  );
}

/** A quick action above a screen: an icon button with its name as its tooltip. */
function Quick(props: { readonly icon: IconName; readonly label: string; readonly onClick: () => void }): ReactNode {
  return (
    <Tooltip content={props.label}>
      <HeaderButton icon={props.icon} label={props.label} onClick={props.onClick} />
    </Tooltip>
  );
}

interface HeaderProps {
  readonly screen: HeaderScreen;
  readonly box: Box;
  readonly view: View;
  readonly selected: boolean;
  readonly hovered: boolean;
  readonly canEdit: boolean;
  readonly pointer: HeaderPointer;
  readonly store: OverlayStore;
  readonly onSelect: (screen: HeaderScreen) => void;
  readonly onAction: (action: HeaderAction, screen: HeaderScreen) => void;
  readonly onRename: (screen: HeaderScreen, title: string) => void;
}

/** The entries of a screen's ⋯ menu: every screen action, the editing ones off without the right to edit. */
function moreEntries(
  p: HeaderProps,
  m: ReturnType<typeof screenHeaderMessages>,
  rename: () => void,
): readonly MenuEntry[] {
  const act = (action: HeaderAction) => () => p.onAction(action, p.screen);
  const locked = !p.canEdit;
  return [
    { kind: "item", id: "rename", label: m.rename, icon: "pencil-simple", onSelect: rename, disabled: locked },
    { kind: "item", id: "duplicate", label: m.duplicate, icon: "copy", onSelect: act("duplicate"), disabled: locked },
    ...(p.screen.entry
      ? []
      : [
          {
            kind: "item" as const,
            id: "entry",
            label: m.makeEntry,
            icon: "house" as const,
            onSelect: act("entry"),
            disabled: locked,
          },
        ]),
    { kind: "separator", id: "view" },
    { kind: "item", id: "settings", label: m.settings, icon: "gear", onSelect: act("settings") },
    { kind: "item", id: "matrix", label: m.matrix, icon: "squares-four", onSelect: act("matrix") },
    { kind: "item", id: "zoom", label: m.zoom, icon: "fit-view", onSelect: act("zoom") },
    { kind: "separator", id: "move" },
    { kind: "item", id: "move", label: m.move, icon: "arrow-up-right", onSelect: act("move"), disabled: locked },
    { kind: "separator", id: "danger" },
    {
      kind: "item",
      id: "delete",
      label: m.delete,
      icon: "trash",
      onSelect: act("delete"),
      danger: true,
      disabled: locked,
    },
  ];
}

/** Whether a pointer event began on one of the header's own controls, which never grip the screen. */
const onControl = (e: PointerEvent<HTMLElement>): boolean =>
  e.target instanceof Element && e.target.closest("[data-no-grip]") !== null;

/**
 * One screen's header bar (MR-2): the page kind, the title (a click renames it in place), Entry for the plugin's entry
 * screen and the route name, quick actions (page settings, every device, zoom) and the ⋯ menu of every screen action.
 * It stays one size at every zoom, as wide as its frame; a narrow frame drops the route, then the quick actions. The
 * bar itself grips the screen: a press selects it, a drag moves it.
 */
function ScreenHeader(p: HeaderProps): ReactNode {
  const { i18n } = useLingui();
  const m = screenHeaderMessages(i18n);
  const kind = pluginCanvasMessages(i18n).pageKind(p.screen.kind);
  const [editing, setEditing] = useState(false);
  const press = useRef<{ x: number; y: number; title: boolean } | null>(null);
  const hits = [titleId(p.screen.id)];
  const place = headerPlace(p.box, p.view);
  const rename = () => {
    p.onSelect(p.screen);
    setEditing(true);
  };
  const down = (e: PointerEvent<HTMLElement>) => {
    if (onControl(e) || editing) {
      return;
    }
    const title = e.target instanceof Element && e.target.closest(".pc-header-title") !== null;
    press.current = (e.button ?? 0) === 0 ? { x: e.clientX, y: e.clientY, title } : null;
    p.pointer.down(e, hits);
  };
  const up = (e: PointerEvent<HTMLElement>) => {
    const was = press.current;
    press.current = null;
    p.pointer.up(e);
    // A click on the title, not a drag, renames the screen in place.
    if (was?.title === true && p.canEdit && Math.hypot(e.clientX - was.x, e.clientY - was.y) <= CLICK_PX) {
      setEditing(true);
    }
  };
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: the bar grips its screen like the canvas title does; its controls are buttons
    <div
      role="toolbar"
      aria-label={m.bar(p.screen.title)}
      className="pc-header"
      data-selected={p.selected || undefined}
      data-hovered={p.hovered || undefined}
      data-editing={editing || undefined}
      style={{ "--x": place.x, "--y": place.y, "--w": place.w } as CSSProperties}
      onPointerDown={down}
      onPointerMove={(e) => press.current !== null && p.pointer.move(e)}
      onPointerUp={(e) => press.current !== null && up(e)}
      onPointerCancel={(e) => {
        press.current = null;
        p.pointer.cancel(e);
      }}
      onPointerEnter={() => p.store.set({ hover: p.screen.root })}
      onPointerLeave={() => press.current === null && p.store.set({ hover: null })}
      onContextMenu={(e) => {
        if (!editing) {
          p.pointer.menu(e, hits);
        }
      }}
    >
      <span className="pc-header-kind" title={kind}>
        <Icon name={KIND_ICON[p.screen.kind]} size="pill" />
      </span>
      {editing ? (
        <TitleField
          title={p.screen.title}
          label={m.titleField}
          onDone={(value) => {
            setEditing(false);
            const text = value?.trim() ?? "";
            if (text !== "" && text !== p.screen.title) {
              p.onRename(p.screen, text);
            }
          }}
        />
      ) : (
        <button
          type="button"
          className="pc-header-title"
          aria-label={m.editTitle(p.screen.title)}
          title={p.canEdit ? m.renameHint : p.screen.title}
          aria-disabled={!p.canEdit || undefined}
          onClick={(e) => {
            // A click from the keyboard (Enter, Space) renames; a pointer click is handled on release.
            if (e.detail === 0 && p.canEdit) {
              rename();
            }
          }}
        >
          <span className="pc-header-name">{p.screen.title}</span>
          {p.canEdit && <Icon name="pencil-simple" size="pill" className="pc-header-pencil" />}
        </button>
      )}
      {p.screen.entry && !editing && (
        <span className="pc-header-entry" title={m.entryHint}>
          {m.entry}
        </span>
      )}
      {routeWorthShowing(p.screen.title, p.screen.route) && !editing && (
        <span className="pc-header-route" title={m.route(p.screen.route ?? "")} dir="ltr">
          {p.screen.route}
        </span>
      )}
      <span className="pc-header-space" />
      <span className="pc-header-quick">
        <Quick icon="gear" label={m.settings} onClick={() => p.onAction("settings", p.screen)} />
        <Quick icon="squares-four" label={m.matrix} onClick={() => p.onAction("matrix", p.screen)} />
        <Quick icon="fit-view" label={m.zoom} onClick={() => p.onAction("zoom", p.screen)} />
      </span>
      <Menu
        onOpenChange={(open) => open && p.onSelect(p.screen)}
        trigger={<HeaderButton icon="dots-three" label={m.more(p.screen.title)} data-more="" />}
        entries={moreEntries(p, m, rename)}
      />
    </div>
  );
}

/**
 * The header bars of the screens (MR-2), over the canvas and under the docks, following every pan and zoom. Each sits
 * just above its frame; the selected screen's is tinted in the selection's colour and the one under the pointer lifts.
 */
export function ScreenHeaders(props: {
  readonly screens: readonly HeaderScreen[];
  readonly store: OverlayStore;
  readonly surface: () => CanvasSurface | null;
  readonly boxOf: (id: string) => Box | undefined;
  readonly selected: ReadonlySet<string>;
  readonly canEdit: boolean;
  readonly pointer: HeaderPointer;
  readonly onSelect: (screen: HeaderScreen) => void;
  readonly onAction: (action: HeaderAction, screen: HeaderScreen) => void;
  readonly onRename: (screen: HeaderScreen, title: string) => void;
}): ReactNode {
  const state = useOverlayState(props.store);
  const surface = props.surface();
  if (surface === null) {
    return null;
  }
  const v = surface.viewport;
  const view = { x: v.x, y: v.y, zoom: v.zoom };
  return (
    <div className="pc-headers" data-busy={state.marquee !== null || undefined}>
      {props.screens.map((s) => {
        const box = props.boxOf(s.root);
        return box === undefined ? null : (
          <ScreenHeader
            key={s.id}
            screen={s}
            box={box}
            view={view}
            selected={props.selected.has(s.root)}
            hovered={state.hover === s.root}
            canEdit={props.canEdit}
            pointer={props.pointer}
            store={props.store}
            onSelect={props.onSelect}
            onAction={props.onAction}
            onRename={props.onRename}
          />
        );
      })}
    </div>
  );
}
