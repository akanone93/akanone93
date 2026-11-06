// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Guides, ScreenStyle } from "@plux/canvas";
import { widget } from "@plux/schema";
import { type ReactNode, type RefObject, useCallback, useMemo, useRef, useState } from "react";
import { type Command, useScopedCommands } from "../../lib/commands/index.ts";
import { useCanvasKeys } from "../app-canvas/use-canvas-keys.ts";
import { metaOf } from "../panels/studio-meta.ts";
import type { NodeProblem } from "../problems/problem-nodes.ts";
import type { BindingMark } from "./binding-marks.ts";
import { CanvasMenu, type MenuAt } from "./canvas-menu.tsx";
import { removeRecipe } from "./clipboard/use-clipboard.tsx";
import type { A11yMarks } from "./interaction-overlay.tsx";
import { InteractionOverlay, type OverlayNodes } from "./interaction-overlay.tsx";
import type { L10nOverlay } from "./l10n-marks.ts";
import type { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import type { Node } from "./plugin-model.ts";
import type { Point } from "./positions.ts";
import type { HandlerStep } from "./quick-actions.ts";
import { type PlacedScreen, screenDocuments } from "./scene.ts";
import type { ScreenRequest } from "./screens-controller.tsx";
import type { EditFn } from "./select/edits.ts";
import { findNode } from "./select/edits.ts";
import type { Box } from "./select/geometry.ts";
import type { OverlayStore } from "./select/overlay-store.ts";
import * as sel from "./select/selection.ts";
import type { SelectionStore } from "./select/store.ts";
import { childNodes, type NodeIndex } from "./select/tree.ts";
import { boxItemOf, type PointerTool, useCanvasPointer } from "./select/use-canvas-pointer.ts";
import type { CanvasSurface } from "./surface.ts";
import type { ScreenPositions } from "./use-screen-positions.ts";

type Messages = ReturnType<typeof pluginCanvasMessages>;

export interface InteractionInput {
  readonly surface: () => CanvasSurface | null;
  readonly canvas: RefObject<HTMLCanvasElement | null>;
  readonly store: SelectionStore;
  readonly selection: sel.Selection;
  readonly index: NodeIndex;
  readonly screenOf: ReadonlyMap<string, string>;
  readonly edit: EditFn;
  readonly canEdit: boolean;
  readonly locked: ReadonlySet<string>;
  /** The screens as placed, and as the canvas shows them (the preview's documents and style). */
  readonly screens: readonly PlacedScreen[];
  readonly shown: readonly PlacedScreen[];
  readonly styled: ScreenStyle;
  /** What each screen's header shows after its name (PC-11), as the page lays the screens out. */
  readonly meta?: (s: PlacedScreen) => string | undefined;
  readonly positions: ScreenPositions;
  readonly onPick: (hits: readonly string[]) => boolean;
  /** The view moved: the URL and the ghosts follow it. */
  readonly onView: () => void;
  readonly fit: () => void;
  readonly zoomBy: (factor: number) => void;
  readonly resetZoom: () => void;
  readonly requestScreen: (kind: ScreenRequest["kind"], screen: string) => void;
  readonly messages: Messages;
  /** The overlay's state: the page makes it, so its surface can route guides to it (`guidesToOverlay`). */
  readonly overlay: OverlayStore;
  /** With the Interact tool, a click on the screens (PC-15). */
  readonly onInteract?: (world: Point, hits: readonly string[]) => void;
  /** The problems pinned to each widget, drawn as dots (PC-16). */
  readonly problems?: ReadonlyMap<string, readonly NodeProblem[]>;
  /** The bound props of each widget, drawn as badges (PC-23). */
  readonly bindings?: ReadonlyMap<string, readonly BindingMark[]>;
  /** The steps each widget's events run, drawn as badges (PC-27). */
  readonly events?: ReadonlyMap<string, readonly HandlerStep[]>;
  /** The widgets of other screens that share a widget's hero tag (PC-29). */
  readonly heroes?: (id: string) => readonly string[];
  /** The Accessibility view's marks, when it is on (PC-31). */
  readonly a11y?: A11yMarks | undefined;
  /** The Localisation view's marks, when it is on (PC-32). */
  readonly l10n?: L10nOverlay | undefined;
  /** A double-click that reaches a text edits it in place (PC-17). */
  readonly onEditText?: (id: string) => void;
  /** A resized width or height snapped to a dimension token near it (PC-18). */
  readonly snapSize?: (value: number) => number;
  /** The tool, when the page holds it. */
  readonly tool?: PointerTool;
  readonly setTool?: (tool: PointerTool) => void;
}

/** Widgets whose space around their child is padding; an `Align` or a `Center` has free space instead (PC-16). */
const PADDED: ReadonlySet<string> = new Set([
  "Padding",
  "AnimatedPadding",
  "SafeArea",
  "Container",
  "AnimatedContainer",
  "Card",
  "SingleChildScrollView",
  "ListView",
  "GridView",
]);

/**
 * Whether node `id` of screen `screen` repeats an item template (PC-24): the canvas draws its items there, so the space
 * around its first item is not padding.
 */
function repeats(screens: readonly PlacedScreen[], screen: string | undefined, id: string): boolean {
  const root = screens.find((s) => s.id === screen)?.root;
  const node =
    root === undefined ? undefined : (findNode(root, id)?.node as { slots?: Record<string, unknown> } | undefined);
  return node?.slots?.["item"] !== undefined;
}

/** The layer names of every node of `roots`, by id; nodes without one are left out. */
function layerNames(roots: readonly (Node | undefined)[]): ReadonlyMap<string, string> {
  const out = new Map<string, string>();
  const visit = (n: Node) => {
    const name = metaOf(n).name;
    if (name !== undefined) {
      out.set(n.id, name);
    }
    for (const k of childNodes(n)) {
      visit(k);
    }
  };
  for (const r of roots) {
    if (r !== undefined) {
      visit(r);
    }
  }
  return out;
}

/** A node's type can be resized on the canvas: its descriptor has `width` and `height`. */
function sizedType(type: string | undefined): boolean {
  const names = type === undefined ? [] : (widget(type)?.props.map((p) => p.name) ?? []);
  return names.includes("width") && names.includes("height");
}

/**
 * The world box of node `id`: its own item's, or — for a node that lays out no box of its own, a parent-data wrapper
 * such as `Positioned`, `Expanded` or `Flexible` — the box around its children's. Undefined while nothing is laid out.
 */
export function nodeBox(
  id: string,
  index: NodeIndex,
  own: (id: string) => Box | undefined,
  depth = 0,
): Box | undefined {
  const box = own(id);
  if (box !== undefined || depth > 8) {
    return box;
  }
  return unionOf((index.children.get(id) ?? []).flatMap((k) => nodeBox(k, index, own, depth + 1) ?? []));
}

/** The smallest box around `boxes`, or undefined for none. */
function unionOf(boxes: readonly Box[]): Box | undefined {
  if (boxes.length === 0) {
    return undefined;
  }
  return {
    l: Math.min(...boxes.map((b) => b.l)),
    t: Math.min(...boxes.map((b) => b.t)),
    r: Math.max(...boxes.map((b) => b.r)),
    b: Math.max(...boxes.map((b) => b.b)),
  };
}

/** Shows the screens at their dragged places while a drag runs: one layout in flight, the latest place waiting. */
function useScreenPreview(input: InteractionInput, overlay: OverlayStore) {
  const latest = useRef(input);
  latest.current = input;
  const state = useRef<{ busy: boolean; next: ReadonlyMap<string, Point> | null }>({ busy: false, next: null });
  return useCallback(
    (moved: ReadonlyMap<string, Point>) => {
      const s = state.current;
      s.next = moved;
      const send = () => {
        const surface = latest.current.surface();
        const at = s.next;
        s.next = null;
        if (surface === null || at === null) {
          s.busy = false;
          return;
        }
        s.busy = true;
        const placed = latest.current.shown.map((x) => {
          const p = at.get(x.id);
          return p === undefined ? x : { ...x, at: p };
        });
        void surface.layoutScreens(screenDocuments(placed, latest.current.meta), latest.current.styled).then(() => {
          overlay.viewChanged();
          send();
        });
      };
      if (!s.busy) {
        send();
      }
    },
    [overlay],
  );
}

/** Delete: the selected widgets are removed as one undo step and their parent selected; a selected screen asks first. */
function useDeleteCommand(input: InteractionInput): () => void {
  const { store, index, screenOf, edit, canEdit, requestScreen, messages: m } = input;
  const remove = useCallback(() => {
    const ids = store.get().ids;
    const root = ids.find((id) => (index.depth.get(id) ?? 1) === 0);
    if (root !== undefined) {
      const screen = screenOf.get(root);
      if (screen !== undefined) {
        requestScreen("delete", screen);
      }
      return;
    }
    const group = `delete-${ids.join(",")}`;
    for (const id of ids) {
      const doc = screenOf.get(id);
      if (doc !== undefined && !input.locked.has(id)) {
        edit(doc, removeRecipe(id), { group });
      }
    }
    store.set(sel.toParent(index, store.get()));
  }, [store, index, screenOf, edit, requestScreen, input.locked]);
  const commands = useMemo<readonly Command[]>(
    () => [
      {
        id: "edit.delete",
        title: m.deleteSelection,
        group: "edit",
        shortcut: "delete",
        permission: null,
        ...(canEdit ? {} : { disabledReason: "read-only" }),
        run: remove,
      },
    ],
    [m.deleteSelection, canEdit, remove],
  );
  useScopedCommands("plugin-delete", commands);
  return remove;
}

/**
 * The plugin canvas's interaction (STU-050, STU-061, STU-067): the pointer and its tools, the overlay that shows the
 * selection, hover, marquee, guides and drops, the screens' drag with its live preview and saved places, the context
 * menu, the tool and view keys (V, H, Space, +, −, Shift+1, Shift+2, Ctrl+0) and Delete. The page renders `element`
 * over its canvas and spreads `handlers` on it.
 */
export function useCanvasInteraction(input: InteractionInput) {
  const { overlay } = input;
  const [ownTool, setOwnTool] = useState<PointerTool>("select");
  // The page may hold the tool, so the screens' style can follow the Interact tool (PC-15).
  const tool = input.tool ?? ownTool;
  const setTool = input.setTool ?? setOwnTool;
  const [menu, setMenu] = useState<MenuAt | null>(null);
  const space = useRef(false);
  const { index, screenOf, screens, store, selection, surface } = input;
  const roots = useMemo(
    () => new Map(screens.flatMap((s) => (s.root === undefined ? [] : [[s.id, s.root.id]]))),
    [screens],
  );
  const names = useMemo(() => layerNames(screens.map((s) => s.root)), [screens]);
  const titles = useMemo(() => new Map(screens.map((s) => [s.id, s.title])), [screens]);
  const boxOf = useCallback(
    (id: string): Box | undefined =>
      nodeBox(id, index, (n) => {
        const item = boxItemOf(index, screenOf, n);
        return item === undefined ? undefined : surface()?.boundsOf?.(item);
      }),
    [index, screenOf, surface],
  );
  const isScreen = useCallback((id: string) => (index.depth.get(id) ?? 1) === 0, [index]);
  const resizable = useCallback(
    (id: string) => input.canEdit && !input.locked.has(id) && sizedType(index.type.get(id)),
    [input.canEdit, input.locked, index],
  );
  const nodes: OverlayNodes = useMemo(
    () => ({
      boxOf,
      isScreen,
      resizable,
      scope: selection.scope,
      ...(input.styled.chrome === undefined ? {} : { frameRadius: input.styled.chrome.radius }),
      childrenOf: (id) => index.children.get(id) ?? [],
      padded: (id) => PADDED.has(index.type.get(id) ?? "") && !repeats(input.screens, screenOf.get(id), id),
      ...(input.problems === undefined ? {} : { problems: input.problems }),
      ...(input.bindings === undefined ? {} : { bindings: input.bindings }),
      ...(input.events === undefined ? {} : { events: input.events }),
      ...(input.heroes === undefined ? {} : { heroes: input.heroes }),
      ...(input.a11y === undefined
        ? {}
        : { a11y: input.a11y, a11yText: { target: input.messages.a11yTarget, contrast: input.messages.a11yContrast } }),
      ...(input.l10n === undefined ? {} : { l10n: input.l10n }),
      nameOf: (id) =>
        isScreen(id) ? (titles.get(screenOf.get(id) ?? "") ?? id) : (names.get(id) ?? index.type.get(id) ?? id),
    }),
    [
      boxOf,
      isScreen,
      resizable,
      selection.scope,
      titles,
      screenOf,
      names,
      index,
      input.styled.chrome,
      input.problems,
      input.bindings,
      input.events,
      input.heroes,
      input.a11y,
      input.l10n,
      input.messages,
      input.screens,
    ],
  );
  const preview = useScreenPreview(input, overlay);
  const view = input.onView;

  const zoomToSelection = useCallback(() => {
    const s = surface();
    const canvas = input.canvas.current;
    const box = unionOf(store.get().ids.flatMap((id) => boxOf(id) ?? []));
    if (s === null || canvas === null || box === undefined) {
      return;
    }
    const width = canvas.clientWidth || window.innerWidth;
    const height = canvas.clientHeight || window.innerHeight;
    const zoom = Math.min(
      4,
      Math.max(0.1, Math.min(width / ((box.r - box.l) * 1.4), height / ((box.b - box.t) * 1.4))),
    );
    s.setViewport((box.l + box.r) / 2 - width / 2 / zoom, (box.t + box.b) / 2 - height / 2 / zoom, zoom);
    view();
  }, [surface, input.canvas, store, boxOf, view]);

  const remove = useDeleteCommand(input);
  const handlers = useCanvasPointer({
    surface,
    canvas: () => input.canvas.current,
    store,
    index,
    edit: input.edit,
    screenOf,
    rootOf: (screen) => roots.get(screen),
    boxOf,
    locked: input.locked,
    canEdit: input.canEdit,
    resizable,
    overlay,
    tool,
    spaceHeld: () => space.current,
    screens: {
      all: () => new Map(screens.map((s) => [s.id, s.at])),
      preview,
      commit: (positions) => {
        if (!input.positions.commit(positions)) {
          preview(new Map(screens.map((s) => [s.id, s.at])));
        }
      },
    },
    onPick: input.onPick,
    onView: view,
    onContextMenu: (at, target) => setMenu({ x: at.x, y: at.y, target }),
    onRenameScreen: (screen) => {
      const root = roots.get(screen);
      if (root !== undefined) {
        store.set({ ids: [root], anchor: root, scope: null });
      }
      input.requestScreen("rename", screen);
    },
    ...(input.onInteract === undefined ? {} : { onInteract: input.onInteract }),
    ...(input.onEditText === undefined ? {} : { onEditText: input.onEditText }),
    ...(input.snapSize === undefined ? {} : { snapSize: input.snapSize }),
  });

  useCanvasKeys({
    canvas: input.canvas,
    space,
    fit: input.fit,
    zoomBy: input.zoomBy,
    resetZoom: input.resetZoom,
    setTool,
    interact: () => setTool("interact"),
    // Escape on the canvas walks up the selection (its own keys); elsewhere it clears. It leaves Interact first.
    clear: () => {
      if (tool === "interact") {
        setTool("select");
      } else if (document.activeElement !== input.canvas.current) {
        store.set(sel.emptySelection);
      }
    },
    fitSelection: zoomToSelection,
  });

  const m = input.messages;
  const element: ReactNode = (
    <>
      {input.positions.failed && (
        <p role="alert" className="absolute inset-x-0 bottom-20 text-center text-body">
          {m.positionsFailed}
        </p>
      )}
      <InteractionOverlay store={overlay} surface={surface} selection={selection} nodes={nodes} />
      <CanvasMenu
        at={menu}
        label={m.canvasMenu}
        text={{
          selectParent: m.selectParent,
          selectChildren: m.selectChildren,
          zoomToSelection: m.zoomToSelection,
          zoomToScreen: m.zoomToScreen,
          fitAll: m.fitAll,
        }}
        actions={{
          selectParent: () => store.set(sel.toParent(index, store.get())),
          selectChildren: () => store.set(sel.enter(index, store.get())),
          zoomToSelection,
          fitAll: input.fit,
        }}
        onClose={() => setMenu(null)}
      />
    </>
  );
  const selectedScreen = selection.anchor === null ? undefined : screenOf.get(selection.anchor);
  return { handlers, element, tool, setTool, overlay, remove, selectedScreen, boxOf };
}

/** Routes a surface's guides to the overlay too, so every drag (inserting from the panel included) shows them. */
export function guidesToOverlay(surface: CanvasSurface, overlay: OverlayStore): CanvasSurface {
  const own = surface.setGuides?.bind(surface);
  surface.setGuides = (guides: Guides | null) => {
    overlay.set({ guides });
    own?.(guides);
  };
  return surface;
}
