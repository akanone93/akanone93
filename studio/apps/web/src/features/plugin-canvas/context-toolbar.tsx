// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { widget } from "@plux/schema";
import { Icon, type IconName, Menu, type MenuEntry } from "@plux/ui";
import type { CSSProperties, ReactElement, ReactNode } from "react";
import {
  addAfterRecipe,
  convertKeepsChildren,
  convertRecipe,
  convertTargets,
  crossAlignmentOf,
  crossAlignments,
  crossAlignRecipe,
  isFlex,
  type SiblingType,
  siblingOf,
  siblingTypes,
  wrapInRecipe,
  wrapTypes,
} from "./context-actions.ts";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";
import type { Node } from "./plugin-model.ts";
import { uuidv7 } from "./screens.ts";
import { duplicateRecipe, type EditFn } from "./select/edits.ts";
import type { Box } from "./select/geometry.ts";
import { type OverlayStore, useOverlayState } from "./select/overlay-store.ts";
import type { SelectionStore } from "./select/store.ts";
import type { NodeIndex } from "./select/tree.ts";
import type { CanvasSurface } from "./surface.ts";
import "./context-toolbar.css";

type Messages = ReturnType<typeof pluginCanvasMessages>;

function ToolButton(props: {
  readonly icon: IconName;
  readonly label: string;
  readonly onClick?: () => void;
  readonly danger?: boolean;
  readonly caret?: boolean;
}): ReactElement {
  return (
    <button
      type="button"
      className="pc-tool"
      aria-label={props.label}
      title={props.label}
      data-danger={props.danger || undefined}
      onClick={props.onClick}
    >
      <Icon name={props.icon} />
      {props.caret === true && <Icon name="caret-down" className="pc-tool-caret" />}
    </button>
  );
}

/** A tool that opens a menu of choices. */
function MenuTool(props: {
  readonly icon: IconName;
  readonly label: string;
  readonly entries: readonly MenuEntry[];
}): ReactElement | null {
  return props.entries.length === 0 ? null : (
    <Menu trigger={<ToolButton icon={props.icon} label={props.label} caret />} entries={props.entries} />
  );
}

/** What the toolbar acts on: the widget, where it stands and its parent. */
interface Target {
  readonly id: string;
  readonly node: Node;
  readonly doc: string;
  readonly parent: Node | undefined;
  readonly inList: boolean;
}

function entriesFor(t: Target, m: Messages, act: (recipe: (d: unknown) => void, select?: string) => void) {
  const kids = (t.node.children?.length ?? 0) + Object.keys(t.node.slots ?? {}).length;
  const wrap: MenuEntry[] = wrapTypes.map((type) => ({
    kind: "item",
    id: `wrap-${type}`,
    label: type,
    onSelect: () => {
      const id = uuidv7();
      act(wrapInRecipe(t.id, id, type), id);
    },
  }));
  const convert: MenuEntry[] = convertTargets(t.node.type).map((type) => ({
    kind: "item",
    id: `convert-${type}`,
    label: type,
    disabled: !convertKeepsChildren(kids, type),
    onSelect: () => act(convertRecipe(t.id, type), t.id),
  }));
  const add: MenuEntry[] = t.inList
    ? siblingTypes.map((type: SiblingType) => ({
        kind: "item",
        id: `add-${type}`,
        label: m.siblingName(type),
        onSelect: () => {
          const node = siblingOf(type, type === "FilledButton" ? m.siblingButton : m.siblingText, uuidv7);
          act(addAfterRecipe(t.id, node), String(node["id"]));
        },
      }))
    : [];
  const parent = t.parent;
  const align: MenuEntry[] =
    parent !== undefined && isFlex(parent.type)
      ? crossAlignments.map((a) => ({
          kind: "item",
          id: `align-${a}`,
          label: m.crossAlign(a),
          disabled: crossAlignmentOf(parent) === a,
          onSelect: () => act(crossAlignRecipe(parent.id, a)),
        }))
      : [];
  return { wrap, convert, add, align };
}

/**
 * The context toolbar of the selected widget (PC-19, STU-067): a bar above it — wrap it in a container, convert it to
 * a related widget, add a sibling after it, align it in its row or column, duplicate it, give it an action, delete it.
 * Each change is one undo step; the new widget (a wrapper, a sibling, a copy) is selected after it.
 */
export function ContextToolbar(props: {
  readonly store: OverlayStore;
  readonly surface: () => CanvasSurface | null;
  readonly selectionStore: SelectionStore;
  readonly ids: readonly string[];
  readonly index: NodeIndex;
  readonly boxOf: (id: string) => Box | undefined;
  readonly nodeOf: (id: string) => Node | undefined;
  readonly docOf: (id: string) => string | undefined;
  readonly edit: EditFn;
  readonly canEdit: boolean;
  readonly locked: ReadonlySet<string>;
  readonly onAction: () => void;
  readonly onDelete: () => void;
  /** Whether the toolbar shows: with the Select tool only. */
  readonly active: boolean;
}): ReactNode {
  const state = useOverlayState(props.store);
  const m = pluginCanvasMessages(useLingui().i18n);
  const surface = props.surface();
  const id = props.ids.length === 1 ? props.ids[0] : undefined;
  const parentId = id === undefined ? undefined : props.index.parent.get(id);
  const node = id === undefined ? undefined : props.nodeOf(id);
  const doc = id === undefined ? undefined : props.docOf(id);
  const box = id === undefined ? undefined : props.boxOf(id);
  const busy = state.ghost !== null || state.marquee !== null;
  if (
    !props.active ||
    surface === null ||
    id === undefined ||
    parentId === null ||
    parentId === undefined ||
    node === undefined ||
    doc === undefined ||
    box === undefined ||
    busy ||
    !props.canEdit ||
    props.locked.has(id)
  ) {
    return null;
  }
  const parent = props.nodeOf(parentId);
  const inList = parent?.children?.some((c) => c.id === id) === true;
  const act = (recipe: (d: unknown) => void, select?: string) => {
    props.edit(doc, recipe, { group: `toolbar:${id}:${Date.now()}` });
    if (select !== undefined) {
      props.selectionStore.set({ ids: [select], anchor: select, scope: props.index.parent.get(id) ?? null });
    }
  };
  const { wrap, convert, add, align } = entriesFor({ id, node, doc, parent, inList }, m, act);
  const v = surface.viewport;
  const above = (box.t - v.y) * v.zoom > 96;
  const style = {
    "--x": ((box.l + box.r) / 2 - v.x) * v.zoom,
    "--y": ((above ? box.t : box.b) - v.y) * v.zoom,
  } as CSSProperties;
  const events = widget(node.type ?? "")?.events ?? [];
  return (
    <div
      className="pc-toolbar"
      role="toolbar"
      aria-label={m.contextToolbar}
      data-place={above ? "above" : "below"}
      style={style}
    >
      <span className="pc-toolbar-name">{node.type}</span>
      <MenuTool icon="frame" label={m.wrapIn} entries={wrap} />
      <MenuTool icon="arrow-clockwise" label={m.convertTo} entries={convert} />
      <MenuTool icon="plus" label={m.addSibling} entries={add} />
      <MenuTool icon="columns" label={m.alignInParent} entries={align} />
      {inList && (
        <ToolButton icon="copy" label={m.duplicateCommand} onClick={() => act(duplicateRecipe(id, () => uuidv7()))} />
      )}
      {events.length > 0 && <ToolButton icon="lightning" label={m.addAction} onClick={props.onAction} />}
      <ToolButton icon="trash" label={m.deleteSelection} danger onClick={props.onDelete} />
    </div>
  );
}
