// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, GlassCapsule, Icon, Menu, Select } from "@plux/ui";
import type { KeyboardEvent, ReactNode } from "react";
import { commandMessages } from "../../lib/commands/commands.messages.ts";
import { useOptionalCommands } from "../../lib/commands/index.ts";
import { devicesMessages } from "../devices/devices.messages.ts";
import { historyMessages } from "../history/history.messages.ts";
import { ProblemCounts } from "../problems/problems-panel.tsx";
import type { Validation } from "../problems/use-validation.ts";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";

export interface ActionBarProps {
  readonly validation: Validation;
  readonly percent: number;
  readonly onZoom: (factor: number) => void;
  readonly onFit: () => void;
  readonly screens: readonly { readonly id: string; readonly title: string }[];
  readonly current: string | undefined;
  readonly onCurrent: (id: string) => void;
  readonly onNewScreen: () => void;
  /** The screen menu of the current screen (rename, move, delete); hidden when absent. */
  /** Opens the history overlay (STU-069); hidden when absent. */
  readonly onHistory?: (() => void) | undefined;
  /** The preview controls: frame, theme, locale and text scale. */
  readonly preview?: ReactNode;
  readonly canSend?: boolean | undefined;
  readonly canPublish?: boolean | undefined;
  /** Opens the publish flow (A1). */
  readonly onPublish: () => void;
  readonly onScreenAction?: ((action: "rename" | "move" | "delete") => void) | undefined;
}

const focusable = "button:not([disabled]), [role=combobox]:not([disabled])";

/** Arrow keys, Home and End move focus along the controls of the bar (roving focus). */
function rove(event: KeyboardEvent<HTMLElement>): void {
  const keys = ["ArrowLeft", "ArrowRight", "Home", "End"];
  if (!keys.includes(event.key) || (event.target as HTMLElement).closest("[role=listbox]") !== null) {
    return;
  }
  const items = [...event.currentTarget.querySelectorAll<HTMLElement>(focusable)];
  const at = items.indexOf(event.target as HTMLElement);
  if (at < 0) {
    return;
  }
  const rtl = getComputedStyle(event.currentTarget).direction === "rtl";
  const step = (event.key === "ArrowRight") !== rtl ? 1 : -1;
  const next =
    event.key === "Home" ? 0 : event.key === "End" ? items.length - 1 : (at + step + items.length) % items.length;
  event.preventDefault();
  items[next]?.focus();
}

/** Undo and redo run the registered `edit.undo` and `edit.redo` commands and stay disabled while none is registered. */
export function ActionBar(props: ActionBarProps) {
  const { i18n } = useLingui();
  const m = pluginCanvasMessages(i18n);
  const history = historyMessages(i18n).open;
  const states = useOptionalCommands()?.states ?? [];
  const command = (id: string) => states.find((s) => s.command.id === id);
  const undo = command("edit.undo");
  const redo = command("edit.redo");
  const send = command("device.send");
  const pair = command("device.pair");
  const devices = devicesMessages(i18n);
  const needs = commandMessages(i18n).missing;
  const publishReason = props.canPublish ? undefined : needs("release.publish");
  return (
    <div className="pointer-events-none absolute inset-x-1/4 top-3 z-10 flex justify-center">
      <GlassCapsule
        role="toolbar"
        label={m.actionBar}
        className="pointer-events-auto h-auto max-w-full flex-nowrap overflow-x-auto py-1 px-3"
        onKeyDown={rove}
      >
        <Button size="sm" variant="ghost" onClick={props.onNewScreen}>
          <Icon name="plus" size="capsule" />
          {m.newScreen}
        </Button>
        {props.screens.length > 0 && (
          <Select
            label={m.currentScreen}
            value={props.current ?? ""}
            options={props.screens.map((s) => ({ value: s.id, label: s.title }))}
            onValueChange={props.onCurrent}
          />
        )}
        {props.onScreenAction !== undefined && props.current !== undefined && (
          <Menu
            trigger={
              <Button size="sm" variant="ghost" aria-label={m.screenMenu}>
                <Icon name="dots-three" size="capsule" />
              </Button>
            }
            entries={[
              { kind: "item", id: "rename", label: m.renameScreen, onSelect: () => props.onScreenAction?.("rename") },
              { kind: "item", id: "move", label: m.moveScreen, onSelect: () => props.onScreenAction?.("move") },
              { kind: "item", id: "delete", label: m.deleteScreen, onSelect: () => props.onScreenAction?.("delete") },
            ]}
          />
        )}
        <Button
          size="sm"
          variant="ghost"
          aria-label={m.undo}
          disabled={undo?.enabled !== true}
          onClick={() => void undo?.command.run()}
        >
          <Icon name="arrow-counter-clockwise" size="capsule" />
        </Button>
        <Button
          size="sm"
          variant="ghost"
          aria-label={m.redo}
          disabled={redo?.enabled !== true}
          onClick={() => void redo?.command.run()}
        >
          <Icon name="arrow-clockwise" size="capsule" />
        </Button>
        <Button size="sm" variant="ghost" aria-label={m.zoomOut} onClick={() => props.onZoom(1 / 1.25)}>
          <Icon name="minus" size="capsule" />
        </Button>
        <output aria-label={m.zoom(props.percent)} className="text-data text-primary">
          {props.percent}%
        </output>
        <Button size="sm" variant="ghost" aria-label={m.zoomIn} onClick={() => props.onZoom(1.25)}>
          <Icon name="plus" size="capsule" />
        </Button>
        <Button size="sm" variant="ghost" onClick={props.onFit}>
          {m.zoomFit}
        </Button>
        <ProblemCounts validation={props.validation} />
        {props.preview}
        <Button
          size="sm"
          variant="ghost"
          disabled={!props.canSend || send === undefined}
          title={props.canSend ? undefined : needs("devsession.create")}
          onClick={() => void send?.command.run()}
        >
          {devices.sendAction}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          disabled={!props.canSend || pair === undefined}
          onClick={() => void pair?.command.run()}
        >
          {devices.pairAction}
        </Button>
        {props.onHistory !== undefined && (
          <Button size="sm" variant="ghost" aria-label={history} onClick={props.onHistory}>
            <Icon name="clock" size="capsule" />
          </Button>
        )}
        <Button
          size="sm"
          variant="primary"
          disabled={!props.canPublish}
          title={publishReason}
          onClick={props.onPublish}
        >
          {m.publish}
        </Button>
      </GlassCapsule>
    </div>
  );
}
