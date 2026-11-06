// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Menu, type MenuEntry } from "@plux/ui";
import type { CSSProperties, ReactElement } from "react";
import { type CommandState, useOptionalCommands } from "../../lib/commands/index.ts";
import type { MenuTarget } from "./select/use-canvas-pointer.ts";

/** Where the menu opens, in canvas pixels, and what it was opened on. */
export interface MenuAt {
  readonly x: number;
  readonly y: number;
  readonly target: MenuTarget;
}

/** The canvas's own actions the menu offers beside the registered commands. */
export interface MenuActions {
  readonly selectParent: () => void;
  readonly selectChildren: () => void;
  readonly zoomToSelection: () => void;
  readonly fitAll: () => void;
}

export interface MenuText {
  readonly selectParent: string;
  readonly selectChildren: string;
  readonly zoomToSelection: string;
  readonly zoomToScreen: string;
  readonly fitAll: string;
}

type Item = Extract<MenuEntry, { kind: "item" }>;

/** The menu entries of a registered command by id: its title, disabled with its reason; none when not registered. */
function commandItem(states: readonly CommandState[], id: string, danger = false): Item[] {
  const state = states.find((s) => s.command.id === id);
  if (state === undefined) {
    return [];
  }
  const disabled = !state.enabled || state.command.disabledReason !== undefined;
  return [
    {
      kind: "item",
      id,
      label: state.command.title,
      onSelect: () => void state.command.run(),
      ...(disabled ? { disabled } : {}),
      ...(danger ? { danger } : {}),
    },
  ];
}

const sep = (id: string): MenuEntry => ({ kind: "separator", id });

/** The entries for a secondary click on `target`: a widget, a screen or the empty canvas. */
export function menuEntries(
  target: MenuTarget,
  states: readonly CommandState[],
  actions: MenuActions,
  m: MenuText,
): MenuEntry[] {
  const c = (id: string, danger = false) => commandItem(states, id, danger);
  const item = (id: string, label: string, onSelect: () => void): Item => ({ kind: "item", id, label, onSelect });
  if (target === "canvas") {
    return [...c("screen.new"), ...c("edit.paste"), sep("view"), item("fit", m.fitAll, actions.fitAll)];
  }
  if (target === "screen") {
    return [
      ...c("screen.rename"),
      ...c("screen.duplicate"),
      ...c("screen.move"),
      sep("edit"),
      ...c("edit.paste"),
      item("children", m.selectChildren, actions.selectChildren),
      item("zoom", m.zoomToScreen, actions.zoomToSelection),
      sep("danger"),
      ...c("screen.delete", true),
    ];
  }
  return [
    ...c("edit.copy"),
    ...c("edit.cut"),
    ...c("edit.paste"),
    ...c("canvas.duplicate"),
    sep("style"),
    ...c("canvas.copyStyle"),
    ...c("canvas.pasteStyle"),
    sep("structure"),
    ...c("canvas.wrap"),
    ...c("canvas.unwrap"),
    sep("select"),
    item("parent", m.selectParent, actions.selectParent),
    item("children", m.selectChildren, actions.selectChildren),
    item("zoom", m.zoomToSelection, actions.zoomToSelection),
    sep("danger"),
    ...c("edit.delete", true),
  ];
}

/**
 * The plugin canvas's context menu (STU-067), opened by a secondary click at the pointer: on a widget its clipboard,
 * structure, selection and delete actions, on a screen the screen's actions, on the empty canvas a new screen, paste and
 * fit. Every action is a registered command or a canvas action, so the menu, the keys and the palette agree.
 */
export function CanvasMenu(props: {
  readonly at: MenuAt | null;
  readonly label: string;
  readonly actions: MenuActions;
  readonly text: MenuText;
  readonly onClose: () => void;
}): ReactElement | null {
  const { at } = props;
  if (at === null) {
    return null;
  }
  return <OpenMenu {...props} at={at} />;
}

/**
 * The open menu: it reads the registered commands itself, so only it re-renders when they change — the page that holds
 * it registers commands of its own and must not follow every registration.
 */
function OpenMenu(props: Parameters<typeof CanvasMenu>[0] & { readonly at: MenuAt }): ReactElement {
  const { at } = props;
  const states = useOptionalCommands()?.states ?? [];
  return (
    <Menu
      key={`${at.target}:${at.x}:${at.y}`}
      open
      onOpenChange={(open) => {
        if (!open) {
          props.onClose();
        }
      }}
      trigger={
        <span
          aria-hidden="true"
          title={props.label}
          className="pc-anchor"
          style={{ "--x": at.x, "--y": at.y } as CSSProperties}
        />
      }
      entries={menuEntries(at.target, states, props.actions, props.text)}
    />
  );
}
