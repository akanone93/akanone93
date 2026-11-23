// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { hotkeysCoreFeature, type ItemInstance, syncDataLoaderFeature } from "@headless-tree/core";
import { useTree } from "@headless-tree/react";
import { useVirtualizer, type Virtualizer } from "@tanstack/react-virtual";
import {
  type CSSProperties,
  type DragEvent,
  type KeyboardEvent,
  type MouseEvent,
  type ReactElement,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { cx, focusRing } from "./cx.ts";
import { Icon } from "./icon.tsx";
import type { IconName } from "./icons.ts";
import { goneNode, type TreeNode, useRebuildOnData } from "./tree.tsx";

/** A level's indent while it fits, in CSS pixels: the caret's width, so each guide stands under its parent's caret. */
const STEP = 20;
/** The least indent a level keeps however deep the tree, in CSS pixels. */
const MIN_STEP = 4;
/** The share of the tree's width, in per cent, the guides of its deepest open row may take before the indent narrows. */
const GUIDE_SHARE = 45;

/**
 * The tree's indent per level (#218): the caret's width while the deepest open row's guides take less than
 * `GUIDE_SHARE` of the tree's width, narrower beyond, so a deep row neither runs off the side nor scrolls the panel
 * sideways. One indent for every row, so each guide still stands under the caret of the row it belongs to.
 */
export function layerStep(deepest: number): string {
  return deepest === 0 ? `${STEP}px` : `clamp(${MIN_STEP}px, calc(${GUIDE_SHARE}cqi / ${deepest}), ${STEP}px)`;
}

/**
 * From this many open rows the tree draws only the rows in view and a few around them (TanStack Virtual, as decided
 * for the Layers tree): a large plugin's tree, fully open, has thousands of rows, and drawing them all took seconds
 * to open and to select a row (#218). Below it every row is drawn, so a small tree reads and searches as a page does.
 */
const VIRTUAL_FROM = 200;
/** Rows drawn past each end of the view, so a fast scroll or a key held down meets drawn rows. */
const OVERSCAN = 12;
/** A row's height before it is measured, in CSS pixels (the comfortable density's row). */
const ROW_GUESS = 28;

/** The nearest ancestor that scrolls vertically: the pane the tree sits in, whose scrolling the rows follow. */
function scrollParentOf(el: HTMLElement | null): HTMLElement | null {
  for (let p = el?.parentElement ?? null; p !== null; p = p.parentElement) {
    const y = getComputedStyle(p).overflowY;
    if (y === "auto" || y === "scroll") {
      return p;
    }
  }
  return null;
}

/**
 * The scrolling pane around the tree and how far down its content the tree starts (the selection summary above it
 * moves it), measured after each render; a figure changes only when it moved.
 */
function usePane(box: { readonly current: HTMLElement | null }): {
  readonly pane: HTMLElement | null;
  readonly offset: number;
} {
  const [pane, setPane] = useState<HTMLElement | null>(null);
  const [offset, setOffset] = useState(0);
  useLayoutEffect(() => {
    const el = box.current;
    const found = scrollParentOf(el);
    if (found !== pane) {
      setPane(found);
    }
    if (el !== null && found !== null) {
      const at = el.getBoundingClientRect().top - found.getBoundingClientRect().top + found.scrollTop;
      if (Math.abs(at - offset) > 0.5) {
        setOffset(at);
      }
    }
  });
  return { pane, offset };
}

/** Where a drawn row stands when the tree draws only the rows in view, and how it reports its height. */
interface RowPlace {
  readonly index: number;
  readonly top: number;
  readonly measure: (el: Element | null) => void;
}

/** A row of the layer tree: a node with its icon, what it shows and an optional mark (the entry screen). */
export interface LayerNode extends TreeNode {
  readonly icon?: IconName;
  /** What the node shows, such as a Text's words or a button's label; dimmed after the name. */
  readonly preview?: string;
  /** A short mark after the name, such as "Entry". */
  readonly badge?: string;
  /** Hidden on the canvas (an editor-only setting): the row is dimmed and shows its eye. */
  readonly hidden?: boolean;
  /** Locked against edits: the row shows its lock. */
  readonly locked?: boolean;
}

/**
 * What a person can do to a layer from its row: rename it (double-click the name, or F2), hide it on the canvas
 * (Ctrl+Shift+H) and lock it (Ctrl+Shift+L). The words come from the caller's catalogue.
 */
export interface LayerActions {
  readonly labels: {
    readonly name: string;
    readonly hide: string;
    readonly show: string;
    readonly lock: string;
    readonly unlock: string;
    readonly hidden: string;
    readonly locked: string;
  };
  readonly onRename: (id: string, name: string) => void;
  readonly onHidden: (id: string, hidden: boolean) => void;
  readonly onLocked: (id: string, locked: boolean) => void;
}

export interface LayerTreeProps {
  readonly nodes: Readonly<Record<string, LayerNode>>;
  readonly rootId: string;
  readonly label: string;
  readonly dir?: "ltr" | "rtl";
  /** The shared selection (F5): the tree shows it, expands to it and scrolls to it. */
  readonly selected: readonly string[];
  readonly onSelect: (id: string, toggle: boolean) => void;
  /** A row was dropped on `target`: before it, or after it (inside it when it is a folder). */
  readonly onDrop?: ((dragged: string, target: string, half: "before" | "after") => void) | undefined;
  /** Why the dragged row cannot go before or after `target`, in a few words; null when it can. */
  readonly dropRefusal?: ((dragged: string, target: string, half: "before" | "after") => string | null) | undefined;
  /** A drag was let go where the row cannot go: `reason` says why, and nothing moved. */
  readonly onRefused?: ((reason: string) => void) | undefined;
  /** Shows only the rows whose name, type or preview contain this text, with their ancestors, all open. */
  readonly filter?: string;
  /** The open rows, when the caller owns them (expand all, collapse all); otherwise the tree keeps its own. */
  readonly expanded?: readonly string[];
  readonly onExpandedChange?: (ids: readonly string[]) => void;
  /** Rename, hide and lock; the rows offer them only when given (read-only editors leave them out). */
  readonly actions?: LayerActions | undefined;
}

const DRAG_TYPE = "application/x-plux-layer";

function parentsOf(nodes: LayerTreeProps["nodes"]): Map<string, string> {
  const parentOf = new Map<string, string>();
  for (const [pid, n] of Object.entries(nodes)) {
    for (const c of n.children ?? []) {
      parentOf.set(c, pid);
    }
  }
  return parentOf;
}

function ancestorsOf(parentOf: ReadonlyMap<string, string>, id: string): string[] {
  const out: string[] = [];
  for (let p = parentOf.get(id); p !== undefined; p = parentOf.get(p)) {
    out.push(p);
  }
  return out;
}

/** The ids of every row with children under `rootId`: what "expand all" opens. */
export function layerFolders(nodes: LayerTreeProps["nodes"], rootId: string): string[] {
  return Object.values(nodes)
    .filter((n) => n.id !== rootId && (n.children?.length ?? 0) > 0)
    .map((n) => n.id);
}

/** The rows a filter keeps: the matches and their ancestors; `null` when there is no filter. */
export function layerMatches(nodes: LayerTreeProps["nodes"], rootId: string, filter: string): Set<string> | null {
  const q = filter.trim().toLowerCase();
  if (q === "") {
    return null;
  }
  const parentOf = parentsOf(nodes);
  const keep = new Set<string>();
  for (const n of Object.values(nodes)) {
    if (n.id === rootId) {
      continue;
    }
    const hay = `${n.name} ${n.type} ${n.preview ?? ""}`.toLowerCase();
    if (hay.includes(q)) {
      keep.add(n.id);
      for (const a of ancestorsOf(parentOf, n.id)) {
        if (a !== rootId) {
          keep.add(a);
        }
      }
    }
  }
  return keep;
}

type Drop = { readonly id: string; readonly half: "before" | "after" } | null;

/**
 * The layer tree on Headless Tree (WAI-ARIA tree, arrows follow reading direction) with the selection owned by the
 * caller: indentation guides, a caret that folds a row, the node's icon, name, preview and mark, a drop line while a
 * row is dragged, and a filter that keeps the matches with their ancestors.
 */
export function LayerTree(props: LayerTreeProps): ReactElement {
  const { nodes, rootId, label, dir = "ltr", selected, onSelect, onDrop, filter = "" } = props;
  const rtl = dir === "rtl";
  const [own, setOwn] = useState<readonly string[]>(() => [...(nodes[rootId]?.children ?? [])]);
  const expanded = props.expanded ?? own;
  const setExpanded = (next: readonly string[]) => {
    if (props.onExpandedChange === undefined) {
      setOwn(next);
    } else {
      props.onExpandedChange(next);
    }
  };
  const keep = useMemo(() => layerMatches(nodes, rootId, filter), [nodes, rootId, filter]);
  // Headless Tree re-renders when its state changes identity: the open rows are kept stable between renders.
  const open = useMemo(() => (keep === null ? [...expanded] : [...keep]), [keep, expanded]);
  const [drop, setDrop] = useState<Drop>(null);
  // The row being dragged: a drag-over cannot read the dragged id, so the tree keeps it to check each target.
  const [dragging, setDragging] = useState<string | null>(null);
  const refusal =
    drop === null || dragging === null ? null : (props.dropRefusal?.(dragging, drop.id, drop.half) ?? null);
  // The refusal at the last row the drag was over: letting go clears the drop line before the drag ends.
  const lastRefusal = useRef<string | null>(null);
  useEffect(() => {
    if (drop !== null) {
      lastRefusal.current = refusal;
    }
  }, [drop, refusal]);
  const box = useRef<HTMLDivElement>(null);
  // Brings a row into view before the tree focuses it from the keyboard; set once the rows are known.
  const reveal = useRef<(id: string) => void>(() => undefined);
  const tree = useTree<LayerNode>({
    rootItemId: rootId,
    getItemName: (item) => item.getItemData().name,
    isItemFolder: (item) => (item.getItemData().children?.length ?? 0) > 0,
    dataLoader: {
      getItem: (id) => nodes[id] ?? goneNode(id),
      getChildren: (id) => [...(nodes[id]?.children ?? [])],
    },
    state: { expandedItems: open },
    setExpandedItems: (u) => setExpanded(typeof u === "function" ? u(open) : u),
    indent: 0,
    hotkeys: {
      expandOrDown: { hotkey: rtl ? "ArrowLeft" : "ArrowRight" },
      collapseOrUp: { hotkey: rtl ? "ArrowRight" : "ArrowLeft" },
    },
    scrollToItem: (item) => reveal.current(item.getId()),
    features: [syncDataLoaderFeature, hotkeysCoreFeature],
  });
  useRebuildOnData(tree, nodes);
  // The open rows, the filter's rows, their keys and the deepest level are worked out again only when the rows change:
  // a fully open tree has thousands, and redoing it on each render made each selection slow (#218).
  const items = tree.getItems();
  const shown = useMemo(() => (keep === null ? items : items.filter((item) => keep.has(item.getId()))), [items, keep]);
  const keyOf = useCallback((i: number) => shown[i]?.getId() ?? i, [shown]);
  const deepest = useMemo(() => items.reduce((d, item) => Math.max(d, item.getItemMeta().level), 0), [items]);
  const { pane, offset } = usePane(box);
  const virtual = shown.length >= VIRTUAL_FROM && pane !== null;
  const rows = useVirtualizer({
    count: virtual ? shown.length : 0,
    getScrollElement: () => pane,
    estimateSize: () => ROW_GUESS,
    overscan: OVERSCAN,
    scrollMargin: offset,
    gap: 1,
    getItemKey: keyOf,
  });
  useLayoutEffect(() => {
    reveal.current = (id) => revealRow(id, shown, virtual ? rows : null, box.current);
  });
  const anchor = selected[selected.length - 1];
  // biome-ignore lint/correctness/useExhaustiveDependencies: runs when the selection or expansion changes, reading the current nodes
  useEffect(() => {
    if (anchor === undefined || keep !== null) {
      return;
    }
    const closed = ancestorsOf(parentsOf(nodes), anchor).filter((a) => a !== rootId && !expanded.includes(a));
    if (closed.length > 0) {
      setExpanded([...new Set([...expanded, ...closed])]);
      return;
    }
    reveal.current(anchor);
  }, [anchor, expanded]);

  const toggle = (id: string, isOpen: boolean) =>
    setExpanded(isOpen ? expanded.filter((x) => x !== id) : [...expanded, id]);

  // The tree's own ref and ours on one element: with only ours, the tree never had its element, never listened for
  // keys, and no arrow key worked in Layers (#218).
  const { ref: treeRef, ...container } = tree.getContainerProps(label);
  const setBox = (el: HTMLDivElement | null) => {
    box.current = el;
    if (typeof treeRef === "function") {
      treeRef(el);
    }
  };
  const row = (item: ItemInstance<LayerNode>, place?: RowPlace) => {
    const id = item.getId();
    const { key: _key, onClick: _click, ...rest } = item.getProps();
    return (
      <LayerRow
        key={id}
        id={id}
        rowProps={rest}
        place={place}
        data={item.getItemData()}
        name={item.getItemName()}
        level={item.getItemMeta().level}
        folder={item.isFolder()}
        open={item.isExpanded()}
        selected={selected.includes(id)}
        drop={drop?.id === id ? drop.half : null}
        refusal={drop?.id === id ? refusal : null}
        onDropAt={(half) => setDrop(half === null ? null : { id, half })}
        onDragging={(on) => {
          const refused = lastRefusal.current;
          lastRefusal.current = null;
          if (!on && refused !== null) {
            props.onRefused?.(refused);
          }
          setDragging(on ? id : null);
          if (!on) {
            setDrop(null);
          }
        }}
        onToggle={() => toggle(id, item.isExpanded())}
        onSelect={(toggleSel) => {
          // The tree's own focus follows a click, which its keys act on: without it, an arrow key after choosing a
          // row moved or opened the row the tree last had in focus (#218).
          item.setFocused();
          onSelect(id, toggleSel);
        }}
        onDrop={onDrop === undefined ? undefined : (dragged, half) => onDrop(dragged, id, half)}
        actions={props.actions}
      />
    );
  };
  const step = { "--layer-step": layerStep(deepest) } as CSSProperties;
  return virtual ? (
    <div
      {...container}
      ref={setBox}
      dir={dir}
      className="@container relative min-w-0 outline-none"
      style={{ ...step, blockSize: rows.getTotalSize() }}
    >
      {rows.getVirtualItems().map((v) => {
        const item = shown[v.index];
        return item === undefined
          ? null
          : row(item, { index: v.index, top: v.start - offset, measure: rows.measureElement });
      })}
    </div>
  ) : (
    <div
      {...container}
      ref={setBox}
      dir={dir}
      className="@container flex min-w-0 flex-col gap-px outline-none"
      style={step}
    >
      {shown.map((item) => row(item))}
    </div>
  );
}

/**
 * Brings row `id` into view: through the virtualizer when the tree draws only the rows in view (the row may not be
 * drawn yet), otherwise by scrolling the drawn row into view.
 */
function revealRow(
  id: string,
  shown: readonly ItemInstance<LayerNode>[],
  rows: Virtualizer<HTMLElement, Element> | null,
  box: HTMLElement | null,
): void {
  if (rows !== null) {
    const index = shown.findIndex((item) => item.getId() === id);
    if (index >= 0) {
      rows.scrollToIndex(index, { align: "auto" });
    }
    return;
  }
  const el = [...(box?.querySelectorAll<HTMLElement>("[data-item-id]") ?? [])].find((e) => e.dataset["itemId"] === id);
  el?.scrollIntoView?.({ block: "nearest" });
}

/** The half of a row the pointer is over: a drop there goes before the row, or after it (into it for a folder). */
function halfOf(e: DragEvent<HTMLDivElement>): "before" | "after" {
  const r = e.currentTarget.getBoundingClientRect();
  return e.clientY < r.top + r.height / 2 ? "before" : "after";
}

/** One row of the layer tree: guides, caret, icon, name, preview and mark, and the drop line while a row is dragged. */
function LayerRow(props: {
  readonly id: string;
  readonly rowProps: Record<string, unknown>;
  /** Where the row stands when only the rows in view are drawn; absent when every row is drawn in order. */
  readonly place?: RowPlace | undefined;
  readonly data: LayerNode;
  readonly name: string;
  readonly level: number;
  readonly folder: boolean;
  readonly open: boolean;
  readonly selected: boolean;
  readonly drop: "before" | "after" | null;
  /** Why the dragged row cannot go where this row's drop line shows; null when it can. */
  readonly refusal: string | null;
  readonly onDropAt: (half: "before" | "after" | null) => void;
  /** This row started (true) or ended (false) being dragged. */
  readonly onDragging: (on: boolean) => void;
  readonly onToggle: () => void;
  readonly onSelect: (toggle: boolean) => void;
  readonly onDrop: ((dragged: string, half: "before" | "after") => void) | undefined;
  readonly actions: LayerActions | undefined;
}): ReactElement {
  const { id, data, level, folder, selected, drop, actions } = props;
  const [renaming, setRenaming] = useState(false);
  const additive = (e: { shiftKey: boolean; ctrlKey: boolean; metaKey: boolean }) =>
    e.shiftKey || e.ctrlKey || e.metaKey;
  // Rename, hide or lock the row, from its shortcut or its toggles.
  const perform = (act: "rename" | "hide" | "lock") => {
    if (act === "rename") {
      setRenaming(true);
    } else if (act === "hide") {
      actions?.onHidden(id, data.hidden !== true);
    } else {
      actions?.onLocked(id, data.locked !== true);
    }
  };
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.target !== e.currentTarget) {
      return;
    }
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      props.onSelect(additive(e));
      return;
    }
    const act = actions === undefined ? undefined : layerShortcut(e);
    if (act !== undefined) {
      e.preventDefault();
      perform(act);
    }
  };
  return (
    <div
      {...props.rowProps}
      ref={rowRef(props.rowProps, props.place)}
      data-index={props.place?.index}
      style={placed(props.place)}
      data-item-id={id}
      role="treeitem"
      tabIndex={(props.rowProps["tabIndex"] as number | undefined) ?? -1}
      aria-selected={selected}
      onKeyDown={onKey}
      draggable={props.onDrop !== undefined && data.locked !== true && !renaming}
      onClick={(e: MouseEvent<HTMLDivElement>) => {
        const hit = e.target instanceof Element ? e.target.closest<HTMLElement>("[data-caret],[data-act]") : null;
        if (hit?.dataset["act"] !== undefined && actions !== undefined) {
          perform(hit.dataset["act"] === "hide" ? "hide" : "lock");
          return;
        }
        if (folder && hit !== null && hit.dataset["caret"] !== undefined) {
          props.onToggle();
          return;
        }
        props.onSelect(additive(e));
      }}
      onDoubleClick={(e) => {
        if (actions !== undefined && e.target instanceof Element && e.target.closest("[data-name]") !== null) {
          setRenaming(true);
        } else if (folder) {
          props.onToggle();
        }
      }}
      onDragStart={(e) => {
        e.dataTransfer.setData(DRAG_TYPE, id);
        props.onDragging(true);
      }}
      onDragEnd={() => props.onDragging(false)}
      onDragOver={(e) => {
        if (props.onDrop !== undefined && e.dataTransfer.types.includes(DRAG_TYPE)) {
          e.preventDefault();
          // A place the row cannot go shows the not-allowed cursor; letting go there moves nothing.
          e.dataTransfer.dropEffect = props.refusal === null ? "move" : "none";
          const half = halfOf(e);
          if (drop !== half) {
            props.onDropAt(half);
          }
        }
      }}
      onDragLeave={() => props.onDropAt(null)}
      onDrop={(e) => {
        props.onDropAt(null);
        const dragged = e.dataTransfer.getData(DRAG_TYPE);
        if (props.onDrop !== undefined && dragged !== "" && props.refusal === null) {
          e.preventDefault();
          props.onDrop(dragged, halfOf(e));
        }
      }}
      className={cx(
        "group relative flex h-(--density-tree-row) min-w-0 cursor-default items-center gap-1.5 rounded-md pe-2 text-start text-sm transition-colors",
        focusRing,
        selected ? "bg-primary/12 text-foreground" : "text-foreground hover:bg-muted",
        drop === "after" &&
          folder &&
          (props.refusal === null ? "ring-2 ring-primary/60" : "ring-2 ring-destructive/70"),
      )}
    >
      <DropLine drop={drop} folder={folder} refusal={props.refusal} />
      <LayerRowContent
        data={data}
        name={props.name}
        level={level}
        folder={folder}
        open={props.open}
        selected={selected}
      />
      {renaming && actions !== undefined && (
        <RenameField
          name={props.name}
          label={actions.labels.name}
          onDone={(name) => {
            setRenaming(false);
            if (name !== null && name !== props.name) {
              actions.onRename(id, name);
            }
          }}
        />
      )}
      {actions !== undefined && <RowToggles data={data} labels={actions.labels} />}
    </div>
  );
}

/** A row's element goes to the tree (which focuses it from the keyboard) and, when placed, to the virtualizer. */
function rowRef(rowProps: Record<string, unknown>, place: RowPlace | undefined) {
  const register = rowProps["ref"];
  return (el: HTMLDivElement | null) => {
    if (typeof register === "function") {
      register(el);
    }
    place?.measure(el);
  };
}

/** A placed row stands at its offset in the tree, across its whole width. */
function placed(place: RowPlace | undefined): CSSProperties | undefined {
  return place === undefined
    ? undefined
    : { position: "absolute", insetInline: 0, top: 0, transform: `translateY(${place.top}px)` };
}

/** The line where a dragged row would land: above the row, or below it unless it is a folder (which takes it). */
/** The line a dragged row would land at, in the primary colour; red with the reason where it cannot go. */
function DropLine(props: {
  readonly drop: "before" | "after" | null;
  readonly folder: boolean;
  readonly refusal: string | null;
}) {
  const { drop, folder, refusal } = props;
  if (drop === null) {
    return null;
  }
  return (
    <>
      {!(drop === "after" && folder) && (
        <span
          aria-hidden="true"
          className={cx(
            "pointer-events-none absolute inset-x-1 h-0.5 rounded-full",
            refusal === null ? "bg-primary" : "bg-destructive",
            drop === "before" ? "top-0" : "bottom-0",
          )}
        />
      )}
      {refusal !== null && (
        <span
          aria-hidden="true"
          className="pointer-events-none absolute end-1 top-1/2 z-10 max-w-[70%] -translate-y-1/2 truncate rounded bg-destructive px-1.5 py-px text-[11px] font-medium leading-4 text-white shadow-sm"
        >
          {refusal}
        </span>
      )}
    </>
  );
}

/** A row's guides, caret, icon, name, preview and mark. */
function LayerRowContent(props: {
  readonly data: LayerNode;
  readonly name: string;
  readonly level: number;
  readonly folder: boolean;
  readonly open: boolean;
  readonly selected: boolean;
}): ReactElement {
  const { data, level, folder, selected } = props;
  const tone = selected ? "text-primary" : level === 0 ? "text-foreground" : "text-muted-foreground";
  return (
    <>
      {level > 0 && (
        // One guide per ancestor, each line under that ancestor's caret (10 px into it at every indent). The padding
        // keeps the last line inside the clip, and the negative margin takes it and the row's gap back. Past the least
        // indent the guides give way before the name does, cut at their end (#218).
        <span
          data-guides=""
          aria-hidden="true"
          className="-me-4 flex min-w-0 shrink-[0.01] self-stretch overflow-hidden pe-2.5"
        >
          {Array.from({ length: level }, (_, i) => (
            <span
              // biome-ignore lint/suspicious/noArrayIndexKey: the guides of a row are positional and never reorder
              key={i}
              className="relative w-(--layer-step) shrink-0 before:absolute before:inset-y-0 before:start-2.5 before:border-s before:border-border"
            />
          ))}
        </span>
      )}
      <span
        data-caret=""
        aria-hidden="true"
        className={cx(
          "flex size-5 shrink-0 items-center justify-center rounded-sm text-muted-foreground",
          folder && "hover:bg-foreground/10 hover:text-foreground",
        )}
      >
        {folder && <Icon name={props.open ? "caret-down" : "caret-right"} size="pill" />}
      </span>
      <Icon name={data.icon ?? "cube"} size="pill" className={tone} />
      <span
        data-name=""
        className={cx(
          "min-w-[6ch] truncate",
          (level === 0 || selected) && "font-medium",
          data.hidden === true && "opacity-50",
        )}
      >
        {props.name}
      </span>
      {/* The preview takes the room left; too narrow to read a word, it is left out rather than cut to a sliver. */}
      <span className="@container min-w-0 flex-1">
        <span className="block truncate text-xs text-muted-foreground @max-[2.5rem]:hidden">{data.preview ?? ""}</span>
      </span>
      {data.badge !== undefined && (
        <span className="shrink-0 rounded-sm bg-primary/12 px-1.5 text-xs font-medium text-primary">{data.badge}</span>
      )}
    </>
  );
}

/** The keyboard's way to a row's actions: F2 renames, Ctrl+Shift+H hides, Ctrl+Shift+L locks (as design tools do). */
function layerShortcut(e: KeyboardEvent): "rename" | "hide" | "lock" | undefined {
  if (e.key === "F2") {
    return "rename";
  }
  if ((e.ctrlKey || e.metaKey) && e.shiftKey) {
    const key = e.key.toLowerCase();
    return key === "h" ? "hide" : key === "l" ? "lock" : undefined;
  }
  return undefined;
}

/** The name field over a row while it is renamed: Enter or leaving it keeps the name, Escape drops it. */
function RenameField({
  name,
  label,
  onDone,
}: {
  readonly name: string;
  readonly label: string;
  readonly onDone: (name: string | null) => void;
}): ReactElement {
  const [value, setValue] = useState(name);
  return (
    <input
      // biome-ignore lint/a11y/noAutofocus: the field opens on the person's own request to rename the row
      autoFocus
      aria-label={label}
      value={value}
      onChange={(e) => setValue(e.target.value)}
      onClick={(e) => e.stopPropagation()}
      onKeyDown={(e) => {
        e.stopPropagation();
        if (e.key === "Enter") {
          onDone(value);
        } else if (e.key === "Escape") {
          onDone(null);
        }
      }}
      onBlur={() => onDone(value)}
      className="absolute inset-y-0.5 end-1 start-12 rounded-sm border border-primary bg-background px-1.5 text-sm outline-none"
    />
  );
}

/**
 * The eye and the lock at a row's end: shown on hover and while set; a click toggles them. Assistive technology hears
 * the state in the row's name and toggles it with the row's shortcuts.
 */
function RowToggles({ data, labels }: { readonly data: LayerNode; readonly labels: LayerActions["labels"] }) {
  const toggle = (act: "hide" | "lock", on: boolean, icon: IconName, state: string) => (
    <span
      data-act={act}
      className={cx(
        "size-5 shrink-0 items-center justify-center rounded-sm text-muted-foreground hover:bg-foreground/10 hover:text-foreground",
        // Shown while set, under the pointer, on the chosen row and on the row in focus; otherwise it takes no room,
        // so names keep it (#218).
        on ? "flex" : "hidden group-hover:flex group-focus-visible:flex group-aria-selected:flex",
      )}
    >
      <Icon name={icon} size="pill" />
      {on && <span className="sr-only">{state}</span>}
    </span>
  );
  return (
    <>
      {toggle("hide", data.hidden === true, data.hidden === true ? "eye-slash" : "eye", labels.hidden)}
      {toggle("lock", data.locked === true, data.locked === true ? "lock" : "lock-open", labels.locked)}
    </>
  );
}
