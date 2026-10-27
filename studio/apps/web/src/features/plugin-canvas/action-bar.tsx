// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, GlassCapsule, Icon, Menu, Select } from "@plux/ui";
import { useOptionalCommands } from "../../lib/commands/index.ts";
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
  readonly onScreenAction?: ((action: "rename" | "move" | "delete") => void) | undefined;
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
  return (
    <div className="absolute inset-x-0 bottom-4 flex justify-center">
      <GlassCapsule label={m.actionBar} className="flex items-center gap-2 px-3">
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
        {props.onHistory !== undefined && (
          <Button size="sm" variant="ghost" aria-label={history} onClick={props.onHistory}>
            <Icon name="clock" size="capsule" />
          </Button>
        )}
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
      </GlassCapsule>
    </div>
  );
}
