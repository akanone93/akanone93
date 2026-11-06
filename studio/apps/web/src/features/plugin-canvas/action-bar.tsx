// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Icon, type IconName, Menu, Popover } from "@plux/ui";
import {
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Select as ShadcnSelect,
} from "@plux/ui/components/select";
import { Separator } from "@plux/ui/components/separator";
import { ToggleGroup, ToggleGroupItem } from "@plux/ui/components/toggle-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@plux/ui/components/tooltip";
import type { KeyboardEvent, ReactNode } from "react";
import { commandMessages } from "../../lib/commands/commands.messages.ts";
import { useOptionalCommands } from "../../lib/commands/index.ts";
import { appCanvasMessages } from "../app-canvas/app-canvas.messages.ts";
import { ZoomControls } from "../app-canvas/canvas-chrome.tsx";
import { devicesMessages } from "../devices/devices.messages.ts";
import { historyMessages } from "../history/history.messages.ts";
import type { Validation } from "../problems/use-validation.ts";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";

export interface ActionBarProps {
  /** The pointer tool (select or hand) and its switch; the bar shows the switch only when given. */
  readonly tool?: "select" | "hand" | "interact";
  readonly onTool?: (tool: "select" | "hand" | "interact") => void;
  /** The Accessibility view (PC-31): focus order, touch targets and contrast marked on the screens. */
  readonly a11y?: boolean | undefined;
  readonly onA11y?: ((on: boolean) => void) | undefined;
  /** The Localisation view (PC-32): missing, untranslated and overflowing texts marked on the screens. */
  readonly l10n?: boolean | undefined;
  readonly onL10n?: ((on: boolean) => void) | undefined;
  /** The validation of the plugin; its counts are part of `status`. */
  readonly validation: Validation;
  readonly percent: number;
  readonly onZoom: (factor: number) => void;
  readonly onFit: () => void;
  readonly screens: readonly { readonly id: string; readonly title: string }[];
  readonly current: string | undefined;
  readonly onCurrent: (id: string) => void;
  readonly onNewScreen: () => void;
  /** Opens the history overlay (STU-069); hidden when absent. */
  readonly onHistory?: (() => void) | undefined;
  /** The preview controls: frame, theme, locale and text scale. */
  readonly preview?: ReactNode;
  readonly canSend?: boolean | undefined;
  readonly canPublish?: boolean | undefined;
  /** Opens the publish flow (A1). */
  readonly onPublish: () => void;
  readonly onScreenAction?: ((action: "rename" | "move" | "delete") => void) | undefined;
  /** Sets the zoom back to 100 %; without it the bar zooms by the factor that gets there. */
  readonly onZoomReset?: (() => void) | undefined;
  /** The start of the bar: the way back, the app and the plugin. */
  readonly identity?: ReactNode;
  /** The end of the bar: selection, save state, problem counts and the theme switch. */
  readonly status?: ReactNode;
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

/** A thin rule between the groups of a bar. */
function Divider(): ReactNode {
  return <Separator orientation="vertical" className="mx-1 h-6" />;
}

/** An icon button of the bar, named by its tooltip and its accessible name. */
function BarButton(props: {
  readonly icon: IconName;
  readonly label: string;
  readonly onClick: () => void;
  readonly disabled?: boolean;
  readonly title?: string | undefined;
  /** A toggle's state; a plain button without it. */
  readonly pressed?: boolean | undefined;
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            size="sm"
            variant="ghost"
            aria-label={props.label}
            aria-pressed={props.pressed}
            data-pressed={props.pressed === true || undefined}
            title={props.title}
            disabled={props.disabled}
            onClick={props.onClick}
          />
        }
      >
        <Icon name={props.icon} size="capsule" />
      </TooltipTrigger>
      <TooltipContent side="bottom">{props.label}</TooltipContent>
    </Tooltip>
  );
}

/**
 * The plugin canvas's top bar (STU-060) in three parts on the canvas's glass: at the start who and where (`identity`),
 * in the centre the tools as one toolbar — the screens (new, the current one, its menu), history (undo, redo), the
 * preview switches in a popover, the history overlay and pairing, then Send to device and Publish side by side — and
 * at the end what the editor reports (`status`). The zoom sits apart, at the bottom start, as on the app canvas.
 */
export function ActionBar(props: ActionBarProps) {
  const { i18n } = useLingui();
  const m = pluginCanvasMessages(i18n);
  const zoomLabels = appCanvasMessages(i18n);
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
  const items = props.screens.map((s) => ({ value: s.id, label: s.title }));
  return (
    <>
      <div className="plugin-topbar pointer-events-none absolute inset-x-3 top-3 z-10 flex items-start justify-between gap-3">
        <div className="plugin-topbar-start pointer-events-auto flex min-w-0">{props.identity}</div>
        <div
          role="toolbar"
          aria-label={m.actionBar}
          onKeyDown={rove}
          className="canvas-panel canvas-bar glass pointer-events-auto flex min-w-0 items-center gap-0.5 overflow-x-auto p-1.5"
        >
          {props.tool !== undefined && props.onTool !== undefined && (
            <>
              <ToggleGroup
                aria-label={zoomLabels.tools}
                value={[props.tool]}
                onValueChange={(v) => {
                  const next = v[0];
                  if (next === "select" || next === "hand" || next === "interact") {
                    props.onTool?.(next);
                  }
                }}
                className="gap-0.5"
              >
                <ToggleGroupItem
                  value="select"
                  aria-label={zoomLabels.toolSelect}
                  title={`${zoomLabels.toolSelect} (V)`}
                  size="sm"
                >
                  <Icon name="cursor" size="capsule" />
                </ToggleGroupItem>
                <ToggleGroupItem
                  value="hand"
                  aria-label={zoomLabels.toolHand}
                  title={`${zoomLabels.toolHand} (H)`}
                  size="sm"
                >
                  <Icon name="hand" size="capsule" />
                </ToggleGroupItem>
                <ToggleGroupItem value="interact" aria-label={m.toolInteract} title={`${m.toolInteract} (I)`} size="sm">
                  <Icon name="play" size="capsule" />
                </ToggleGroupItem>
              </ToggleGroup>
              {props.onA11y !== undefined && (
                <BarButton
                  icon="accessibility"
                  label={m.a11yView}
                  title={m.a11yView}
                  pressed={props.a11y === true}
                  onClick={() => props.onA11y?.(props.a11y !== true)}
                />
              )}
              {props.onL10n !== undefined && (
                <BarButton
                  icon="translate"
                  label={m.l10nView}
                  title={m.l10nView}
                  pressed={props.l10n === true}
                  onClick={() => props.onL10n?.(props.l10n !== true)}
                />
              )}
              <Divider />
            </>
          )}
          <BarButton icon="plus" label={m.newScreen} onClick={props.onNewScreen} />
          {props.screens.length > 0 && (
            <ShadcnSelect
              items={items}
              modal={false}
              value={props.current ?? ""}
              onValueChange={(v) => {
                if (typeof v === "string") {
                  props.onCurrent(v);
                }
              }}
            >
              <SelectTrigger aria-label={m.currentScreen} size="sm" className="screen-picker">
                <Icon name="device-mobile" size="pill" />
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {items.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {o.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </ShadcnSelect>
          )}
          {props.onScreenAction !== undefined && props.current !== undefined && (
            <Menu
              trigger={
                <Button size="sm" variant="ghost" aria-label={m.screenMenu} title={m.screenMenu}>
                  <Icon name="pencil-simple" size="capsule" />
                </Button>
              }
              entries={[
                { kind: "item", id: "rename", label: m.renameScreen, onSelect: () => props.onScreenAction?.("rename") },
                { kind: "item", id: "move", label: m.moveScreen, onSelect: () => props.onScreenAction?.("move") },
                { kind: "separator", id: "danger" },
                { kind: "item", id: "delete", label: m.deleteScreen, onSelect: () => props.onScreenAction?.("delete") },
              ]}
            />
          )}
          <Divider />
          <BarButton
            icon="arrow-counter-clockwise"
            label={m.undo}
            disabled={undo?.enabled !== true}
            onClick={() => void undo?.command.run()}
          />
          <BarButton
            icon="arrow-clockwise"
            label={m.redo}
            disabled={redo?.enabled !== true}
            onClick={() => void redo?.command.run()}
          />
          <Divider />
          {props.preview !== undefined && (
            <Popover
              title={m.preview}
              trigger={
                <Button size="sm" variant="ghost">
                  <Icon name="monitor" size="capsule" />
                  {m.preview}
                  <Icon name="caret-down" size="pill" />
                </Button>
              }
            >
              <div className="flex flex-col gap-3">{props.preview}</div>
            </Popover>
          )}
          {props.onHistory !== undefined && <BarButton icon="history" label={history} onClick={props.onHistory} />}
          <BarButton
            icon="plug"
            label={devices.pairAction}
            disabled={!props.canSend || pair === undefined}
            onClick={() => void pair?.command.run()}
          />
          <Divider />
          <span className="flex items-center gap-1.5">
            <Button
              size="sm"
              variant="secondary"
              aria-label={devices.sendAction}
              disabled={!props.canSend || send === undefined}
              title={props.canSend ? devices.sendAction : needs("devsession.create")}
              onClick={() => void send?.command.run()}
            >
              <Icon name="paper-plane-tilt" size="capsule" />
              {m.send}
            </Button>
            <Button
              size="sm"
              variant="primary"
              disabled={!props.canPublish}
              title={publishReason}
              onClick={props.onPublish}
            >
              <Icon name="rocket-launch" size="capsule" />
              {m.publish}
            </Button>
          </span>
        </div>
        <div className="plugin-topbar-end pointer-events-auto flex min-w-0 justify-end">{props.status}</div>
      </div>
      <ZoomControls
        zoom={props.percent / 100}
        messages={zoomLabels}
        className="plugin-zoom pointer-events-auto absolute bottom-3 z-10"
        onZoomIn={() => props.onZoom(1.25)}
        onZoomOut={() => props.onZoom(1 / 1.25)}
        onReset={() => (props.onZoomReset === undefined ? props.onZoom(100 / props.percent) : props.onZoomReset())}
        onFit={props.onFit}
      />
    </>
  );
}
