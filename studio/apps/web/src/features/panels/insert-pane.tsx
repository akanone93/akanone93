// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { WidgetDescriptor } from "@plux/schema";
import { Icon, type IconName } from "@plux/ui";
import { type CSSProperties, type DragEvent, memo, type ReactElement, useState, useSyncExternalStore } from "react";
import { createPortal } from "react-dom";
import { AnimatedList } from "../../components/animated-list.tsx";
import { groupOf, insertGroups, kindIcon, kindOf, typeIcon, type WidgetKind, widgetKinds } from "./catalogue.ts";
import { DockSection } from "./dock-tabs.tsx";
import { panelsMessages } from "./panels.messages.ts";

export const WIDGET_DRAG_TYPE = "application/x-plux-widget";

const RECENT_KEY = "plux.insert.recent";
const RECENT_MAX = 6;

/** The widgets inserted last, in this browser: a per-viewer convenience that may be missing or blocked. */
export function readRecent(): string[] {
  try {
    const raw = globalThis.localStorage?.getItem(RECENT_KEY);
    const list: unknown = raw === null || raw === undefined ? [] : JSON.parse(raw);
    return Array.isArray(list) ? list.filter((x): x is string => typeof x === "string").slice(0, RECENT_MAX) : [];
  } catch {
    return [];
  }
}

function remember(type: string): string[] {
  const next = [type, ...readRecent().filter((t) => t !== type)].slice(0, RECENT_MAX);
  try {
    globalThis.localStorage?.setItem(RECENT_KEY, JSON.stringify(next));
  } catch {
    // Storage blocked or full: the list simply stays as it was.
  }
  return next;
}

type Section = {
  readonly key: string;
  readonly label: string;
  readonly icon: IconName;
  readonly list: WidgetDescriptor[];
};

/** Where the card goes, in window pixels: beside the dock at the tile's height, upwards near the window's foot. */
interface Spot {
  readonly x: number;
  readonly y: number;
  readonly side: "after" | "before";
  readonly up: boolean;
}

type Shown = { readonly d: WidgetDescriptor; readonly glyph: string; readonly spot: Spot } | null;

const CARD_ROOM = 220;
const CARD_GAP = 10;

/** The spot beside the dock for the tile `el`: the side away from the dock, flipping up when the tile is low. */
export function spotOf(el: Element): Spot {
  const tile = el.getBoundingClientRect();
  const dock = (el.closest(".plugin-dock") ?? el).getBoundingClientRect();
  const rtl = getComputedStyle(el).direction === "rtl";
  const up = tile.top + CARD_ROOM > window.innerHeight;
  return {
    x: rtl ? dock.left - CARD_GAP : dock.right + CARD_GAP,
    y: up ? tile.bottom : tile.top,
    side: rtl ? "before" : "after",
    up,
  };
}

/** The widget under the pointer or the focus, in a store of its own: only the details card redraws as it changes. */
function createDetails() {
  let shown: Shown = null;
  const listeners = new Set<() => void>();
  return {
    get: () => shown,
    set: (next: Shown) => {
      if (next?.d !== shown?.d) {
        shown = next;
        for (const l of listeners) {
          l();
        }
      }
    },
    subscribe: (l: () => void) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
  };
}

/** A widget's own icon where the layer tree has one, else its kind's; components show the component icon. */
function glyphIcon(d: WidgetDescriptor): IconName {
  return groupOf(d) === "layer1" ? typeIcon(d.type) : "component";
}

/**
 * The details of the widget under the pointer or the focus, beside the dock at the tile's height (upwards for the
 * last tiles), so it never covers the gallery. It lives at the document's end: the dock's glass would clip it.
 */
function TileDetails({ store }: { readonly store: ReturnType<typeof createDetails> }): ReactElement | null {
  const m = panelsMessages(useLingui().i18n);
  const shown = useSyncExternalStore(store.subscribe, store.get, store.get);
  if (shown === null) {
    return null;
  }
  const { d, glyph, spot } = shown;
  return createPortal(
    <aside
      aria-hidden="true"
      className="gallery-details"
      data-side={spot.side}
      data-up={spot.up || undefined}
      style={{ "--x": spot.x, "--y": spot.y } as CSSProperties}
    >
      <div className="flex items-center gap-2">
        <span className="gallery-glyph" data-kind={glyph}>
          <Icon name={glyphIcon(d)} />
        </span>
        <span className="text-sm font-semibold">{d.type}</span>
        <span className="ms-auto rounded-sm bg-muted px-1.5 text-muted-foreground">{d.category}</span>
      </div>
      {d.description !== "" && <p className="text-muted-foreground">{d.description}</p>}
      <p className="tabular-nums text-muted-foreground">{m.tileFacts(d.props.length, d.events.length)}</p>
      <p className="text-muted-foreground">{m.tileHint}</p>
    </aside>,
    document.body,
  );
}

/**
 * The Insert pane (STU-061): a foldable Categories section of kind tiles (closed at first), the widgets used last, and a
 * section per kind of tiles that animate as the search or the kind narrows them; the widget under the pointer or the
 * focus shows its details at the foot of the pane; a tile inserts on a click and can be dragged onto a screen.
 */
export const InsertPane = memo(function InsertPane({
  found,
  all,
  canEdit,
  note,
  onInsert,
}: {
  /** The widgets the search keeps. */
  readonly found: readonly WidgetDescriptor[];
  /** Every widget, for the category counts and the recent list. */
  readonly all: readonly WidgetDescriptor[];
  readonly canEdit: boolean;
  readonly note: string | null;
  readonly onInsert: (d: WidgetDescriptor) => void;
}): ReactElement {
  const m = panelsMessages(useLingui().i18n);
  const [details] = useState(createDetails);
  const [kind, setKind] = useState<WidgetKind | null>(null);
  const [recent, setRecent] = useState<string[]>(readRecent);
  const kindLabel: Record<WidgetKind, string> = {
    layout: m.kindLayout,
    content: m.kindContent,
    input: m.kindInput,
    interaction: m.kindInteraction,
    navigation: m.kindNavigation,
    motion: m.kindMotion,
    platform: m.kindPlatform,
    more: m.kindMore,
  };
  const basics = found.filter((d) => groupOf(d) === "layer1");
  const sections: Section[] = [
    ...widgetKinds.map((k) => ({
      key: k as string,
      label: kindLabel[k],
      icon: kindIcon[k] as IconName,
      list: kind === null || kind === k ? basics.filter((d) => kindOf(d) === k) : [],
    })),
    ...insertGroups
      .filter((g) => g !== "layer1")
      .map((g) => ({
        key: g as string,
        label: m[g],
        icon: (g === "layer2" ? "component" : "puzzle-piece") as IconName,
        list: kind === null ? found.filter((d) => groupOf(d) === g) : [],
      })),
  ].filter((x) => x.list.length > 0);
  const recentList = recent.flatMap((t) => all.find((d) => d.type === t) ?? []);
  const insert = (d: WidgetDescriptor) => {
    onInsert(d);
    if (canEdit) {
      setRecent(remember(d.type));
    }
  };
  const startDrag = (e: DragEvent, d: WidgetDescriptor) => {
    e.dataTransfer.setData(WIDGET_DRAG_TYPE, d.type);
    e.dataTransfer.effectAllowed = "copy";
  };
  const tile = (d: WidgetDescriptor, glyph: string) => (
    <button
      type="button"
      className="gallery-tile"
      disabled={!canEdit}
      draggable={canEdit}
      onDragStart={(e) => startDrag(e, d)}
      aria-label={m.insert(d.type)}
      onClick={() => insert(d)}
      onPointerEnter={(e) => details.set({ d, glyph, spot: spotOf(e.currentTarget) })}
      onPointerLeave={() => details.set(null)}
      onFocus={(e) => details.set({ d, glyph, spot: spotOf(e.currentTarget) })}
      onBlur={() => details.set(null)}
    >
      <span className="gallery-glyph" data-kind={glyph} aria-hidden="true">
        <Icon name={glyphIcon(d)} />
      </span>
      <span className="gallery-name">{d.type}</span>
    </button>
  );
  const grid = (list: readonly WidgetDescriptor[], glyph: (d: WidgetDescriptor) => string, label: string) => (
    <AnimatedList
      items={list}
      keyOf={(d) => d.type}
      label={label}
      className="gallery-grid"
      render={(d) => tile(d, glyph(d))}
    />
  );
  const counts = new Map(widgetKinds.map((k) => [k, basics.filter((d) => kindOf(d) === k).length]));
  return (
    <div className="flex flex-col gap-1">
      {!canEdit && <p className="dock-note">{m.readOnly}</p>}
      {note !== null && (
        <p role="status" className="dock-note">
          {note}
        </p>
      )}
      <DockSection label={m.categories} icon="squares-four" count={widgetKinds.length} defaultOpen={false}>
        {/* biome-ignore lint/a11y/useSemanticElements: a labelled group of filter toggles, not a form's fieldset */}
        <div role="group" aria-label={m.categories} className="gallery-categories">
          {widgetKinds.map((k) => (
            <button
              key={k}
              type="button"
              className="gallery-category"
              aria-pressed={kind === k}
              onClick={() => setKind(kind === k ? null : k)}
            >
              <span className="gallery-glyph" data-kind={k} aria-hidden="true">
                <Icon name={kindIcon[k]} />
              </span>
              <span className="gallery-name">{kindLabel[k]}</span>
              <span className="gallery-category-count">{counts.get(k) ?? 0}</span>
            </button>
          ))}
        </div>
      </DockSection>
      {kind !== null && (
        <button type="button" className="gallery-filter" aria-label={m.showAllKinds} onClick={() => setKind(null)}>
          <Icon name={kindIcon[kind]} size="pill" />
          {m.showingKind(kindLabel[kind])}
          <Icon name="x" size="pill" />
        </button>
      )}
      {kind === null && found.length === all.length && recentList.length > 0 && (
        <DockSection label={m.recent} icon="clock" count={recentList.length}>
          {grid(recentList, (d) => (groupOf(d) === "layer1" ? kindOf(d) : groupOf(d)), m.recent)}
        </DockSection>
      )}
      {sections.length === 0 && <p className="dock-note">{m.noMatches}</p>}
      {sections.map((x) => (
        <DockSection key={x.key} label={x.label} icon={x.icon} count={x.list.length}>
          {grid(x.list, () => x.key, x.label)}
        </DockSection>
      ))}
      <TileDetails store={details} />
    </div>
  );
});
