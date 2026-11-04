// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { hotkeysCoreFeature, syncDataLoaderFeature } from "@headless-tree/core";
import { useTree } from "@headless-tree/react";
import {
  type DragEvent,
  type KeyboardEvent,
  type MouseEvent,
  type ReactElement,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { cx, focusRing } from "./cx.ts";
import { Icon } from "./icon.tsx";
import type { IconName } from "./icons.ts";
import type { TreeNode } from "./tree.tsx";

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
  const box = useRef<HTMLDivElement>(null);
  const tree = useTree<LayerNode>({
    rootItemId: rootId,
    getItemName: (item) => item.getItemData().name,
    isItemFolder: (item) => (item.getItemData().children?.length ?? 0) > 0,
    dataLoader: {
      getItem: (id) => nodes[id] as LayerNode,
      getChildren: (id) => [...(nodes[id]?.children ?? [])],
    },
    state: { expandedItems: open },
    setExpandedItems: (u) => setExpanded(typeof u === "function" ? u(open) : u),
    indent: 0,
    hotkeys: {
      expandOrDown: { hotkey: rtl ? "ArrowLeft" : "ArrowRight" },
      collapseOrUp: { hotkey: rtl ? "ArrowRight" : "ArrowLeft" },
    },
    features: [syncDataLoaderFeature, hotkeysCoreFeature],
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
    const row = [...(box.current?.querySelectorAll<HTMLElement>("[data-item-id]") ?? [])].find(
      (e) => e.dataset["itemId"] === anchor,
    );
    row?.scrollIntoView?.({ block: "nearest" });
  }, [anchor, expanded]);

  const toggle = (id: string, isOpen: boolean) =>
    setExpanded(isOpen ? expanded.filter((x) => x !== id) : [...expanded, id]);

  return (
    <div {...tree.getContainerProps(label)} ref={box} dir={dir} className="flex flex-col gap-px outline-none">
      {tree.getItems().map((item) => {
        const id = item.getId();
        if (keep !== null && !keep.has(id)) {
          return null;
        }
        const { key: _key, onClick: _click, ...rest } = item.getProps();
        return (
          <LayerRow
            key={id}
            id={id}
            rowProps={rest}
            data={item.getItemData()}
            name={item.getItemName()}
            level={item.getItemMeta().level}
            folder={item.isFolder()}
            open={item.isExpanded()}
            selected={selected.includes(id)}
            drop={drop?.id === id ? drop.half : null}
            onDropAt={(half) => setDrop(half === null ? null : { id, half })}
            onToggle={() => toggle(id, item.isExpanded())}
            onSelect={(toggleSel) => onSelect(id, toggleSel)}
            onDrop={onDrop === undefined ? undefined : (dragged, half) => onDrop(dragged, id, half)}
            actions={props.actions}
          />
        );
      })}
    </div>
  );
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
  readonly data: LayerNode;
  readonly name: string;
  readonly level: number;
  readonly folder: boolean;
  readonly open: boolean;
  readonly selected: boolean;
  readonly drop: "before" | "after" | null;
  readonly onDropAt: (half: "before" | "after" | null) => void;
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
      onDragStart={(e) => e.dataTransfer.setData(DRAG_TYPE, id)}
      onDragOver={(e) => {
        if (props.onDrop !== undefined && e.dataTransfer.types.includes(DRAG_TYPE)) {
          e.preventDefault();
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
        if (props.onDrop !== undefined && dragged !== "") {
          e.preventDefault();
          props.onDrop(dragged, halfOf(e));
        }
      }}
      className={cx(
        "group relative flex h-(--density-tree-row) min-w-0 cursor-default items-center gap-1.5 rounded-md pe-2 text-start text-sm transition-colors",
        focusRing,
        selected ? "bg-primary/12 text-foreground" : "text-foreground hover:bg-muted",
        drop === "after" && folder && "ring-2 ring-primary/60",
      )}
    >
      <DropLine drop={drop} folder={folder} />
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

/** The line where a dragged row would land: above the row, or below it unless it is a folder (which takes it). */
function DropLine({ drop, folder }: { readonly drop: "before" | "after" | null; readonly folder: boolean }) {
  if (drop === null || (drop === "after" && folder)) {
    return null;
  }
  return (
    <span
      aria-hidden="true"
      className={cx(
        "pointer-events-none absolute inset-x-1 h-0.5 rounded-full bg-primary",
        drop === "before" ? "top-0" : "bottom-0",
      )}
    />
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
      {Array.from({ length: level }, (_, i) => (
        <span
          // biome-ignore lint/suspicious/noArrayIndexKey: the guides of a row are positional and never reorder
          key={i}
          aria-hidden="true"
          className="relative w-3.5 shrink-0 self-stretch before:absolute before:inset-y-0 before:start-2 before:border-s before:border-border"
        />
      ))}
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
          "shrink-0 truncate",
          (level === 0 || selected) && "font-medium",
          data.hidden === true && "opacity-50",
        )}
      >
        {props.name}
      </span>
      <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">{data.preview ?? ""}</span>
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
  const toggle = (act: "hide" | "lock", on: boolean, icon: IconName, title: string, state: string) => (
    <span
      data-act={act}
      title={title}
      className={cx(
        "flex size-5 shrink-0 items-center justify-center rounded-sm text-muted-foreground hover:bg-foreground/10 hover:text-foreground",
        on ? "opacity-100" : "opacity-0 group-hover:opacity-100 group-aria-selected:opacity-100",
      )}
    >
      <Icon name={icon} size="pill" />
      {on && <span className="sr-only">{state}</span>}
    </span>
  );
  return (
    <>
      {toggle(
        "hide",
        data.hidden === true,
        data.hidden === true ? "eye-slash" : "eye",
        data.hidden === true ? labels.show : labels.hide,
        labels.hidden,
      )}
      {toggle(
        "lock",
        data.locked === true,
        data.locked === true ? "lock" : "lock-open",
        data.locked === true ? labels.unlock : labels.lock,
        labels.locked,
      )}
    </>
  );
}
