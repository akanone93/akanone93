// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { titleId } from "@plux/canvas";
import { Icon, type IconName, Menu, type MenuEntry } from "@plux/ui";
import {
  type ButtonHTMLAttributes,
  type CSSProperties,
  type KeyboardEvent,
  type MouseEvent,
  memo,
  type PointerEvent,
  type ReactNode,
  type Ref,
  useEffect,
  useMemo,
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
export type HeaderAction = "matrix" | "zoom" | "duplicate" | "entry" | "move" | "delete";

/**
 * The canvas's pointer handlers (STU-067). A header hands them its presses as a hit on its screen's title, so a press
 * selects the screen, a drag moves it with the canvas's snapping, and a secondary click opens the canvas's screen menu.
 */
export interface HeaderPointer {
  readonly down: (e: PointerEvent<HTMLElement>, hits: readonly string[]) => void;
  readonly move: (e: PointerEvent<HTMLElement>) => void;
  readonly up: (e: PointerEvent<HTMLElement>) => void;
  readonly cancel: (e: PointerEvent<HTMLElement>) => void;
  /** The bar lost the pointer mid-drag without a release: the drag ends as a cancel. */
  readonly lost: (e: PointerEvent<HTMLElement>) => void;
  readonly menu: (e: MouseEvent<HTMLElement>, hits: readonly string[]) => void;
}

/** The icon of each page kind (SCH-022): a phone for a screen, a window for a dialog, rows for a sheet. */
const KIND_ICON: Readonly<Record<PageKind, IconName>> = {
  screen: "device-mobile",
  dialog: "frame",
  bottomSheet: "rows",
  fullscreenDialog: "maximize",
};

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

/** The header's ⋯ button; it never grips the screen. A menu hands its trigger props and ref through `rest`. */
function MoreButton({
  label,
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  readonly label: string;
  readonly ref?: Ref<HTMLButtonElement>;
}): ReactNode {
  return (
    <button {...rest} type="button" className="pc-header-button" aria-label={label} data-more="" data-no-grip="">
      <Icon name="dots-three" size="pill" />
    </button>
  );
}

/** What a header asks the page for, through handlers that keep their identity while the page re-renders. */
interface HeaderHandlers {
  readonly pointer: HeaderPointer;
  readonly store: OverlayStore;
  readonly onSelect: (screen: HeaderScreen) => void;
  readonly onAction: (action: HeaderAction, screen: HeaderScreen) => void;
  readonly onRename: (screen: HeaderScreen, title: string) => void;
}

interface HeaderProps {
  readonly screen: HeaderScreen;
  /** The frame's top start and width in the headers' plane, in CSS pixels (the plane follows the pan). */
  readonly x: number;
  readonly y: number;
  readonly w: number;
  /** Free room to the frame's end before the next screen's header, in CSS pixels: a long title reads into it (#215). */
  readonly room?: number;
  readonly selected: boolean;
  readonly hovered: boolean;
  readonly canEdit: boolean;
  readonly handlers: HeaderHandlers;
}

/** The entries of a screen's ⋯ menu: the actions a screen has, the editing ones off without the right to edit. */
function moreEntries(
  p: HeaderProps,
  m: ReturnType<typeof screenHeaderMessages>,
  rename: () => void,
): readonly MenuEntry[] {
  const act = (action: HeaderAction) => () => p.handlers.onAction(action, p.screen);
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
    { kind: "item", id: "zoom", label: m.zoom, icon: "fit-view", onSelect: act("zoom") },
    { kind: "item", id: "matrix", label: m.matrix, icon: "squares-four", onSelect: act("matrix") },
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

/**
 * The ⋯ menu, mounted only once it is opened (#214): forty screens' menus and their floating parts were rendered on
 * every pointer move of a drag; at rest a header holds one plain button.
 */
function MoreMenu(props: {
  readonly label: string;
  readonly entries: () => readonly MenuEntry[];
  readonly onOpen: () => void;
}): ReactNode {
  const [open, setOpen] = useState(false);
  if (!open) {
    return (
      <MoreButton
        label={props.label}
        aria-haspopup="menu"
        onClick={() => {
          props.onOpen();
          setOpen(true);
        }}
      />
    );
  }
  return <Menu open onOpenChange={setOpen} trigger={<MoreButton label={props.label} />} entries={props.entries()} />;
}

/**
 * Whether an event is not the bar's own to grip the screen with: it began on one of the bar's controls, or outside the
 * bar's element altogether. React passes an open menu's events up to the bar through the portal; a press on a menu
 * item used to start a drag of the screen, which took the pointer, so the item never got its click (#230).
 */
const notOurs = (e: PointerEvent<HTMLElement> | MouseEvent<HTMLElement>): boolean =>
  !(e.target instanceof Node && e.currentTarget.contains(e.target)) ||
  (e.target instanceof Element && e.target.closest("[data-no-grip]") !== null);

/**
 * One screen's header (MR-2, #227, #230): the page kind, the title, Entry for the plugin's entry screen, the route name
 * when it says more than the title, and the ⋯ menu of the screen's actions. Like a frame's label in a design tool it is
 * quiet text above its frame at rest and a small chip under the pointer or when selected; it stays one size at every
 * zoom, as wide as its frame, and shows its whole title under the pointer. The whole bar, the title included, grips the
 * screen: a press selects it, a drag moves it; renaming is in the menu. It renders again only when its own screen,
 * place, selection or hover changes.
 */
const ScreenHeader = memo(function ScreenHeader(p: HeaderProps): ReactNode {
  const { i18n } = useLingui();
  const m = screenHeaderMessages(i18n);
  const kind = pluginCanvasMessages(i18n).pageKind(p.screen.kind);
  const [editing, setEditing] = useState(false);
  const press = useRef(false);
  const h = p.handlers;
  const hits = [titleId(p.screen.id)];
  const rename = () => {
    h.onSelect(p.screen);
    setEditing(true);
  };
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: the bar grips its screen like the canvas title does; its controls are buttons
    <div
      role="toolbar"
      aria-label={m.bar(p.screen.title)}
      className="pc-header"
      data-screen={p.screen.id}
      data-selected={p.selected || undefined}
      data-hovered={p.hovered || undefined}
      data-editing={editing || undefined}
      style={{ "--x": p.x, "--y": p.y, "--w": p.w, "--room": p.room ?? 0 } as CSSProperties}
      onPointerDown={(e) => {
        if (notOurs(e) || editing) {
          return;
        }
        press.current = true;
        h.pointer.down(e, hits);
      }}
      onPointerMove={(e) => press.current && h.pointer.move(e)}
      onPointerUp={(e) => {
        if (press.current) {
          press.current = false;
          h.pointer.up(e);
        }
      }}
      onPointerCancel={(e) => {
        press.current = false;
        h.pointer.cancel(e);
      }}
      onLostPointerCapture={(e) => {
        if (press.current) {
          press.current = false;
          h.pointer.lost(e);
        }
      }}
      onPointerEnter={() => h.store.set({ hover: p.screen.root })}
      onPointerLeave={() => !press.current && h.store.set({ hover: null })}
      onContextMenu={(e) => {
        if (!editing && !notOurs(e)) {
          h.pointer.menu(e, hits);
        }
      }}
    >
      <span className="pc-header-kind" aria-label={kind} role="img">
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
              h.onRename(p.screen, text);
            }
          }}
        />
      ) : (
        <span className="pc-header-name">{p.screen.title}</span>
      )}
      {p.screen.entry && !editing && <span className="pc-header-entry">{m.entry}</span>}
      {routeWorthShowing(p.screen.title, p.screen.route) && !editing && (
        <span className="pc-header-route" dir="ltr">
          {p.screen.route}
        </span>
      )}
      <span className="pc-header-space" />
      <MoreMenu
        label={m.more(p.screen.title)}
        onOpen={() => h.onSelect(p.screen)}
        entries={() => moreEntries(p, m, rename)}
      />
    </div>
  );
});

/**
 * The header bars of the screens (MR-2), over the canvas and under the docks, following every pan and zoom. Each sits
 * just above its frame. The bars lie on one plane that a pan only shifts, so a pan or a drag renders no bar but the
 * moved screen's; a zoom places them again.
 */
/** A header's height above its frame, with its gap, in CSS pixels (screen-header.css). */
const HEADER_BAND = 30;
/** The most room a title takes beyond its frame, and the space it leaves before the next header, in CSS pixels. */
const MAX_ROOM = 240;
const ROOM_GAP = 10;

/**
 * The free room after `box`'s frame for its title, in CSS pixels at `zoom` (#215): up to the nearest frame to its end
 * whose header shares the band, less a small gap. At low zoom a frame is narrower than its title; the gap beside it is
 * free, so the title reads into it instead of shrinking to "Ap…".
 */
export function headerRoom(box: Box, boxes: readonly (Box | undefined)[], zoom: number): number {
  const band = HEADER_BAND / zoom;
  let free = MAX_ROOM / zoom;
  for (const o of boxes) {
    if (o !== undefined && o !== box && o.l >= box.r && Math.abs(o.t - box.t) < band) {
      free = Math.min(free, o.l - box.r);
    }
  }
  return Math.max(0, Math.round(free * zoom - ROOM_GAP));
}

/**
 * How far below the view's top a screen brought into view by its header's focus stands, in CSS pixels: below the top
 * bar, as Layers reveals one.
 */
const FOCUS_TOP = 120;

/**
 * The view that shows a screen whose header took focus outside the view or under a dock (`covered`) — by Tab, or a menu
 * handing focus back — at the same zoom: the screen centred across, its top under its header; null when the header is
 * in view already. The layer is clipped, not scrolled, so the canvas moves instead (#229).
 */
export function viewToShow(
  box: Box,
  view: { readonly x: number; readonly y: number; readonly zoom: number },
  size: { readonly width: number; readonly height: number },
  covered = false,
): { readonly x: number; readonly y: number } | null {
  const z = view.zoom;
  const l = (box.l - view.x) * z;
  const r = (box.r - view.x) * z;
  const t = (box.t - view.y) * z;
  if (!covered && r > 0 && l < size.width && t - HEADER_BAND >= 0 && t < size.height) {
    return null;
  }
  return { x: (box.l + box.r) / 2 - size.width / 2 / z, y: box.t - FOCUS_TOP / z };
}

/** Brings into view the screen whose header holds `target`, when the header is outside the view. */
function revealFocused(
  target: EventTarget | null,
  layer: HTMLElement,
  p: {
    readonly screens: readonly HeaderScreen[];
    readonly surface: () => CanvasSurface | null;
    readonly boxOf: (id: string) => Box | undefined;
  },
): void {
  const surface = p.surface();
  const header = target instanceof Element ? target.closest<HTMLElement>("[data-screen]") : null;
  const screen = p.screens.find((s) => s.id === header?.dataset["screen"]);
  const box = screen === undefined ? undefined : p.boxOf(screen.root);
  if (surface === null || header === null || box === undefined) {
    return;
  }
  // A dock or bar over the header hides it as surely as the view's edge does.
  const r = header.getBoundingClientRect();
  const hit = document.elementFromPoint(r.left + Math.min(r.width / 2, 12), r.top + r.height / 2);
  const covered = hit === null || !header.contains(hit);
  const to = viewToShow(box, surface.viewport, { width: layer.clientWidth, height: layer.clientHeight }, covered);
  if (to !== null) {
    surface.setViewport(to.x, to.y, surface.viewport.zoom);
  }
}

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
  // The bars' handlers keep one identity for the page's lifetime and call the latest ones, so a re-render of the page
  // never re-renders a bar on their account.
  const latest = useRef(props);
  latest.current = props;
  const handlers = useMemo<HeaderHandlers>(
    () => ({
      pointer: {
        down: (e, hits) => latest.current.pointer.down(e, hits),
        move: (e) => latest.current.pointer.move(e),
        up: (e) => latest.current.pointer.up(e),
        cancel: (e) => latest.current.pointer.cancel(e),
        lost: (e) => latest.current.pointer.lost(e),
        menu: (e, hits) => latest.current.pointer.menu(e, hits),
      },
      store: props.store,
      onSelect: (s) => latest.current.onSelect(s),
      onAction: (a, s) => latest.current.onAction(a, s),
      onRename: (s, t) => latest.current.onRename(s, t),
    }),
    [props.store],
  );
  // Focus on a header outside the view brings its screen into view (#229); bound once, it reads the latest props.
  const focusIn = useMemo(() => {
    let bound: HTMLDivElement | null = null;
    const on = (e: Event) => {
      if (bound !== null) {
        revealFocused(e.target, bound, latest.current);
      }
    };
    return (el: HTMLDivElement | null) => {
      bound?.removeEventListener("focusin", on);
      bound = el;
      el?.addEventListener("focusin", on);
    };
  }, []);
  const surface = props.surface();
  if (surface === null) {
    return null;
  }
  const v = surface.viewport;
  const z = v.zoom;
  const boxes = props.screens.map((s) => props.boxOf(s.root));
  return (
    <div ref={focusIn} className="pc-headers" data-busy={state.marquee !== null || undefined}>
      <div className="pc-headers-plane" style={{ transform: `translate(${-v.x * z}px, ${-v.y * z}px)` }}>
        {props.screens.map((s, i) => {
          const box = boxes[i];
          return box === undefined ? null : (
            <ScreenHeader
              key={s.id}
              screen={s}
              x={box.l * z}
              y={box.t * z}
              w={Math.max(0, (box.r - box.l) * z)}
              room={headerRoom(box, boxes, z)}
              selected={props.selected.has(s.root)}
              hovered={state.hover === s.root}
              canEdit={props.canEdit}
              handlers={handlers}
            />
          );
        })}
      </div>
    </div>
  );
}
