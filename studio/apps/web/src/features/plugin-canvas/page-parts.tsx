// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { frameId } from "@plux/canvas";
import { Icon, type IconName } from "@plux/ui";
import { type CSSProperties, type ReactNode, useCallback, useMemo, useState } from "react";
import type { PreviewRender } from "../preview/switches.ts";
import {
  addPartRecipe,
  type DrawerSide,
  drawersOf,
  drawersOfSelection,
  missingParts,
  type PagePart,
  partTemplate,
  selectedPageScaffold,
  shownDrawers,
} from "./page-parts.ts";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import type { Node } from "./plugin-model.ts";
import { uuidv7 } from "./screens.ts";
import type { EditFn } from "./select/edits.ts";
import { type OverlayStore, useOverlayState } from "./select/overlay-store.ts";
import type { Selection } from "./select/selection.ts";
import type { SelectionStore } from "./select/store.ts";
import type { CanvasSurface } from "./surface.ts";
import "./page-parts.css";

/** Where a chip stands on the frame: its anchor in world units and which of its edges sits there. */
interface Spot {
  readonly x: number;
  readonly y: number;
  readonly anchor: "top" | "bottom" | "start" | "end" | "bottom-end";
}

const ICONS: Readonly<Record<PagePart, IconName>> = {
  appBar: "rows",
  bottomNavigationBar: "navigation-arrow",
  floatingActionButton: "plus",
  drawer: "panel-start-open",
};

/**
 * The parts a selected page's scaffold can still have, offered where they would stand on its frame (PC-12): **App
 * bar** at the top below the status bar, **Navigation bar** at the bottom above the home indicator, **Action button**
 * at the bottom end, **Drawer** at the start edge; a scaffold with a drawer offers to show or hide it. The chips
 * follow the view as the overlay does; a click adds the part as one undo step.
 */
export function PageParts(props: {
  readonly store: OverlayStore;
  readonly surface: () => CanvasSurface | null;
  readonly screen: string | undefined;
  readonly scaffold: Node | undefined;
  /** The device's status bar and home indicator, so the chips sit inside the page. */
  readonly insets: { readonly top: number; readonly bottom: number };
  /** The previewed direction: the drawer's start and the button's end follow it. */
  readonly rtl: boolean;
  readonly canEdit: boolean;
  readonly shown: ReadonlyMap<string, DrawerSide>;
  readonly onAdd: (part: PagePart) => void;
  readonly onToggleDrawer: (side: DrawerSide) => void;
}): ReactNode {
  useOverlayState(props.store);
  const m = pluginCanvasMessages(useLingui().i18n);
  const surface = props.surface();
  const { scaffold, screen } = props;
  const frame = screen === undefined ? undefined : surface?.boundsOf?.(frameId(screen));
  if (surface === null || scaffold === undefined || frame === undefined) {
    return null;
  }
  const v = surface.viewport;
  const at = (s: Spot): CSSProperties =>
    ({ "--x": (s.x - v.x) * v.zoom, "--y": (s.y - v.y) * v.zoom }) as CSSProperties;
  const mid = (frame.l + frame.r) / 2;
  const start = props.rtl ? frame.r - 12 : frame.l + 12;
  const end = props.rtl ? frame.l + 16 : frame.r - 16;
  const spots: Readonly<Record<PagePart, Spot>> = {
    appBar: { x: mid, y: frame.t + props.insets.top + 12, anchor: "top" },
    bottomNavigationBar: { x: mid, y: frame.b - props.insets.bottom - 12, anchor: "bottom" },
    floatingActionButton: { x: end, y: frame.b - props.insets.bottom - 64, anchor: "bottom-end" },
    drawer: { x: start, y: (frame.t + frame.b) / 2, anchor: "start" },
  };
  const labels: Readonly<Record<PagePart, string>> = {
    appBar: m.addAppBar,
    bottomNavigationBar: m.addNavigationBar,
    floatingActionButton: m.addActionButton,
    drawer: m.addDrawer,
  };
  const missing = props.canEdit ? missingParts(scaffold) : [];
  const drawers = drawersOf(scaffold);
  const open = props.shown.get(scaffold.id);
  return (
    <fieldset className="pc-parts" aria-label={m.pageParts} data-rtl={props.rtl || undefined}>
      {missing.map((part) => (
        <button
          key={part}
          type="button"
          className="pc-part"
          data-anchor={spots[part].anchor}
          style={at(spots[part])}
          onClick={() => props.onAdd(part)}
        >
          <Icon name={ICONS[part]} />
          <span>{labels[part]}</span>
        </button>
      ))}
      {drawers.map((side, i) => {
        const shown = open === side;
        const spot: Spot =
          side === "drawer"
            ? { x: start, y: (frame.t + frame.b) / 2 + i * 40, anchor: "start" }
            : { x: props.rtl ? frame.l + 12 : frame.r - 12, y: (frame.t + frame.b) / 2 + 40, anchor: "end" };
        return (
          <button
            key={side}
            type="button"
            className="pc-part"
            data-kind="toggle"
            data-anchor={spot.anchor}
            aria-pressed={shown}
            style={at(spot)}
            onClick={() => props.onToggleDrawer(side)}
          >
            <Icon name={shown ? "eye-slash" : "eye"} />
            <span>{m.drawerToggle(side, shown)}</span>
          </button>
        );
      })}
    </fieldset>
  );
}

/** The drawers the canvas shows open (PC-12): the ones the user showed or hid, and the one the selection is in. */
export function useShownDrawers(
  roots: readonly (Node | undefined)[],
  selected: readonly string[],
): {
  readonly drawers: ReadonlyMap<string, DrawerSide>;
  readonly toggle: (scaffold: string, side: DrawerSide) => void;
} {
  const [toggles, setToggles] = useState<ReadonlyMap<string, DrawerSide | null>>(new Map());
  const drawers = useMemo(() => shownDrawers(toggles, drawersOfSelection(roots, selected)), [toggles, roots, selected]);
  const toggle = useCallback(
    (scaffold: string, side: DrawerSide) =>
      setToggles((t) => new Map(t).set(scaffold, drawers.get(scaffold) === side ? null : side)),
    [drawers],
  );
  return { drawers, toggle };
}

/**
 * The page parts of the selected page (PC-12): the chips, and what a click on one does — the part is added to the
 * page's scaffold as one undo step and selected; a drawer is shown or hidden.
 */
export function PagePartsHost(props: {
  readonly store: OverlayStore;
  readonly surface: () => CanvasSurface | null;
  readonly roots: readonly (Node | undefined)[];
  readonly screens: readonly { readonly id: string; readonly title: string }[];
  readonly selection: Selection;
  readonly selectionStore: SelectionStore;
  readonly screenOf: ReadonlyMap<string, string>;
  readonly edit: EditFn;
  readonly canEdit: boolean;
  readonly look: PreviewRender;
  readonly drawers: ReadonlyMap<string, DrawerSide>;
  readonly onToggleDrawer: (scaffold: string, side: DrawerSide) => void;
}): ReactNode {
  const m = pluginCanvasMessages(useLingui().i18n);
  const scaffold = selectedPageScaffold(props.roots, props.selection.ids);
  const screen = scaffold === undefined ? undefined : props.screenOf.get(scaffold.id);
  const addPart = (part: PagePart) => {
    if (scaffold === undefined || screen === undefined || !props.canEdit) {
      return;
    }
    const texts = {
      title: props.screens.find((s) => s.id === screen)?.title ?? "",
      home: m.partHome,
      search: m.partSearch,
      profile: m.partProfile,
      settings: m.partSettings,
    };
    const node = partTemplate(part, texts, uuidv7);
    props.edit(screen, addPartRecipe(scaffold.id, part, node), { group: "insert" });
    const id = String(node["id"]);
    props.selectionStore.set({ ids: [id], anchor: id, scope: scaffold.id });
  };
  return (
    <PageParts
      store={props.store}
      surface={props.surface}
      screen={screen}
      scaffold={scaffold}
      insets={{ top: props.look.device.padding.top, bottom: props.look.device.padding.bottom }}
      rtl={props.look.direction === "rtl"}
      canEdit={props.canEdit}
      shown={props.drawers}
      onAdd={addPart}
      onToggleDrawer={(side) => scaffold !== undefined && props.onToggleDrawer(scaffold.id, side)}
    />
  );
}
