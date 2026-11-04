// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { type App, OrgService } from "@plux/api";
import { Icon, type IconName, Menu } from "@plux/ui";
import { Button } from "@plux/ui/components/button";
import { Kbd } from "@plux/ui/components/kbd";
import { Separator } from "@plux/ui/components/separator";
import { ToggleGroup, ToggleGroupItem } from "@plux/ui/components/toggle-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@plux/ui/components/tooltip";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import type { ReactElement, ReactNode } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { CardVisual } from "../apps/card-visual.tsx";
import { ThemeButton } from "../shell/theme-button.tsx";
import type { appCanvasMessages } from "./app-canvas.messages.ts";
import type { SceneBox } from "./app-canvas.scene.ts";
import type { CanvasTool } from "./use-canvas-gestures.ts";

type Messages = ReturnType<typeof appCanvasMessages>;

/** A floating panel of the canvas: frosted glass over the scene, which runs on underneath it. */
export function Panel({
  className = "",
  children,
  ...rest
}: { readonly className?: string; readonly children: ReactNode } & Omit<
  React.ComponentProps<"div">,
  "className" | "children"
>): ReactElement {
  return (
    <div {...rest} className={`canvas-panel glass ${className}`}>
      {children}
    </div>
  );
}

/** An icon button of a panel with its name and shortcut in a tooltip; the name is also its accessible name. */
export function ToolButton({
  icon,
  label,
  shortcut,
  onClick,
  disabled = false,
  variant = "ghost",
  text = false,
}: {
  readonly icon: IconName;
  readonly label: string;
  readonly shortcut?: string;
  readonly onClick: () => void;
  readonly disabled?: boolean;
  readonly variant?: "ghost" | "default";
  /** Shows the label beside the icon too. */
  readonly text?: boolean;
}): ReactElement {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant={variant}
            size={text ? "sm" : "icon-sm"}
            aria-label={label}
            disabled={disabled}
            onClick={onClick}
          />
        }
      >
        <Icon name={icon} {...(text ? { "data-icon": "inline-start" as const } : {})} />
        {text && <span>{label}</span>}
      </TooltipTrigger>
      <TooltipContent side="bottom" className="flex items-center gap-2">
        {label}
        {shortcut !== undefined && <Kbd>{shortcut}</Kbd>}
      </TooltipContent>
    </Tooltip>
  );
}

/** Top start: the way back to the organisation, and the app this canvas belongs to with its number of plugins. */
export function AppBar({
  org,
  app,
  plugins,
  messages: m,
}: {
  readonly org: string;
  readonly app: App | undefined;
  readonly plugins: number;
  readonly messages: Messages;
}): ReactElement {
  const { session } = useApp();
  // The organisations the shell lists too, from the same cache: the way back names the organisation.
  const orgs = useQuery({
    queryKey: ["organisations"],
    queryFn: () => createClient(OrgService, session.transport).listOrganizations({}),
  });
  const orgName = orgs.data?.organizations.find((o) => o.key === org || o.id === org)?.name ?? org;
  return (
    <Panel className="canvas-bar flex gap-2 p-1.5 pe-4">
      <Tooltip>
        <TooltipTrigger
          render={
            <Link
              to="/o/$org"
              params={{ org }}
              aria-label={m.back(orgName)}
              className="inline-flex size-8 items-center justify-center rounded-lg hover:bg-muted"
            />
          }
        >
          <Icon name="arrow-left" />
        </TooltipTrigger>
        <TooltipContent side="bottom">{m.back(orgName)}</TooltipContent>
      </Tooltip>
      <Separator orientation="vertical" className="h-6" />
      {app !== undefined && <CardVisual app={app} compact />}
      <div className="flex min-w-0 flex-col">
        <h1 className="truncate text-sm font-semibold">{app?.name ?? ""}</h1>
        <p className="truncate text-xs text-muted-foreground">{m.pluginCount(plugins)}</p>
      </div>
    </Panel>
  );
}

/** Top centre: the tools and the canvas-wide actions. */
export function CanvasToolbar({
  tool,
  onTool,
  messages: m,
  selected,
  onNew,
  onFrame,
  onArrange,
  onTrash,
}: {
  readonly tool: CanvasTool;
  readonly onTool: (tool: CanvasTool) => void;
  readonly messages: Messages;
  readonly selected: number;
  readonly onNew: () => void;
  readonly onFrame: () => void;
  readonly onArrange: () => void;
  readonly onTrash: () => void;
}): ReactElement {
  return (
    <Panel role="toolbar" aria-label={m.toolbar} className="canvas-bar flex gap-1 p-1.5">
      <ToggleGroup
        aria-label={m.tools}
        value={[tool]}
        onValueChange={(v) => {
          const next = v[0];
          if (next === "select" || next === "hand") {
            onTool(next);
          }
        }}
        className="gap-0.5"
      >
        <ToggleGroupItem value="select" aria-label={m.toolSelect} size="sm">
          <Icon name="cursor" />
        </ToggleGroupItem>
        <ToggleGroupItem value="hand" aria-label={m.toolHand} size="sm">
          <Icon name="hand" />
        </ToggleGroupItem>
      </ToggleGroup>
      <Separator orientation="vertical" className="mx-1 h-6" />
      <ToolButton icon="squares-four" label={m.newFrame} shortcut="F" disabled={selected === 0} onClick={onFrame} />
      <ToolButton icon="sparkle" label={m.autoLayout} onClick={onArrange} />
      <ToolButton icon="trash" label={m.trash} onClick={onTrash} />
      <Separator orientation="vertical" className="mx-1 h-6" />
      <ToolButton icon="plus" label={m.newPlugin} variant="default" text onClick={onNew} />
    </Panel>
  );
}

/** The labels of the zoom controls; both canvases pass the same ones, so the controls read alike. */
export type ZoomLabels = Pick<
  Messages,
  "zoomControls" | "zoomOut" | "zoomIn" | "zoomReset" | "zoomPercent" | "zoomToFit"
>;

/** Bottom start of both canvases: zoom out, the zoom (a click resets it to 100%), zoom in, and zoom to fit. */
export function ZoomControls({
  zoom,
  messages: m,
  onZoomIn,
  onZoomOut,
  onReset,
  onFit,
  className = "",
}: {
  readonly zoom: number;
  readonly messages: ZoomLabels;
  readonly onZoomIn: () => void;
  readonly onZoomOut: () => void;
  readonly onReset: () => void;
  readonly onFit: () => void;
  readonly className?: string;
}): ReactElement {
  const percent = Math.round(zoom * 100);
  return (
    <Panel role="group" aria-label={m.zoomControls} className={`canvas-bar flex gap-0.5 p-1.5 ${className}`}>
      <ToolButton icon="minus" label={m.zoomOut} shortcut="-" onClick={onZoomOut} />
      <Button
        variant="ghost"
        size="sm"
        className="w-14 tabular-nums"
        aria-label={m.zoomReset(percent)}
        onClick={onReset}
      >
        {m.zoomPercent(percent)}
      </Button>
      <ToolButton icon="plus" label={m.zoomIn} shortcut="+" onClick={onZoomIn} />
      <Separator orientation="vertical" className="mx-0.5 h-5" />
      <ToolButton icon="fit-view" label={m.zoomToFit} shortcut="⇧1" onClick={onFit} />
    </Panel>
  );
}

/**
 * Top end of both canvases: what the canvas reports (the selection, a result), the save state, the problem counts and
 * the light and dark switch, in one glass bar whose parts keep their width.
 */
export function CanvasStatus({ children }: { readonly children: ReactNode }): ReactElement {
  return (
    <Panel className="canvas-bar canvas-status flex items-center gap-2 p-1.5 ps-3">
      {children}
      <Separator orientation="vertical" className="h-5" />
      <ThemeButton />
    </Panel>
  );
}

/** The actions on the selected plugin, at the foot of its detail panel: open it, and everything else it offers. */
export function PluginActions({
  selectedId,
  shownId,
  messages: m,
  archived,
  onOpen,
  onDialogue,
  onMove,
  onArchive,
  onDelete,
}: {
  /** The selected plugin and the one the detail panel shows; the actions are there only when they are the same. */
  readonly selectedId: string | undefined;
  readonly shownId: string | undefined;
  readonly messages: Messages;
  readonly archived: boolean;
  readonly onOpen: () => void;
  readonly onDialogue: (kind: "settings" | "duplicate" | "entry") => void;
  readonly onMove: () => void;
  readonly onArchive: () => void;
  readonly onDelete: () => void;
}): ReactElement | null {
  if (selectedId === undefined || selectedId !== shownId) {
    return null;
  }
  return (
    // biome-ignore lint/a11y/useSemanticElements: a labelled group of buttons, not a form's fieldset
    <div role="group" aria-label={m.pluginActions} className="flex flex-col gap-2 border-t pt-3">
      <Button onClick={onOpen}>
        <Icon name="arrow-up-right" data-icon="inline-start" />
        {m.openPlugin}
      </Button>
      <div className="grid grid-cols-2 gap-2">
        <Button variant="outline" size="sm" onClick={() => onDialogue("settings")}>
          <Icon name="gear" data-icon="inline-start" />
          {m.settings}
        </Button>
        <Button variant="outline" size="sm" onClick={() => onDialogue("duplicate")}>
          <Icon name="copy" data-icon="inline-start" />
          {m.duplicate}
        </Button>
        <Button variant="outline" size="sm" onClick={() => onDialogue("entry")}>
          <Icon name="house" data-icon="inline-start" />
          {m.setEntry}
        </Button>
        <Button variant="outline" size="sm" onClick={onMove}>
          <Icon name="arrows-up-down" data-icon="inline-start" />
          {m.moveSelection}
        </Button>
        <Button variant="outline" size="sm" onClick={onArchive}>
          <Icon name={archived ? "arrow-counter-clockwise" : "folder"} data-icon="inline-start" />
          {archived ? m.restore : m.archive}
        </Button>
        <Button variant="destructive" size="sm" onClick={onDelete}>
          <Icon name="trash" data-icon="inline-start" />
          {m.deletePlugin}
        </Button>
      </div>
    </div>
  );
}

/**
 * A plugin's context menu (STU-034), opened by a secondary click on its box: open it and everything else it offers,
 * at the pointer.
 */
export function PluginContextMenu({
  at,
  messages: m,
  onClose,
  onOpen,
  onDialogue,
  onMove,
  onArchive,
  onDelete,
}: {
  readonly at: { readonly box: SceneBox; readonly x: number; readonly y: number } | null;
  readonly messages: Messages;
  readonly onClose: () => void;
  readonly onOpen: (box: SceneBox) => void;
  readonly onDialogue: (kind: "settings" | "duplicate" | "entry") => void;
  readonly onMove: (box: SceneBox) => void;
  readonly onArchive: () => void;
  readonly onDelete: () => void;
}): ReactElement | null {
  if (at === null) {
    return null;
  }
  const { box } = at;
  return (
    <Menu
      key={`${box.id}:${at.x}:${at.y}`}
      open
      onOpenChange={(open) => {
        if (!open) {
          onClose();
        }
      }}
      trigger={
        <span
          aria-hidden="true"
          className="canvas-anchor"
          style={{ "--x": at.x, "--y": at.y } as React.CSSProperties}
        />
      }
      entries={[
        { kind: "item", id: "open", label: m.openPlugin, onSelect: () => onOpen(box) },
        { kind: "item", id: "settings", label: m.settings, onSelect: () => onDialogue("settings") },
        { kind: "item", id: "duplicate", label: m.duplicate, onSelect: () => onDialogue("duplicate") },
        { kind: "item", id: "entry", label: m.setEntry, onSelect: () => onDialogue("entry") },
        { kind: "separator", id: "place" },
        { kind: "item", id: "move", label: m.moveSelection, onSelect: () => onMove(box) },
        { kind: "item", id: "archive", label: box.archived ? m.restore : m.archive, onSelect: onArchive },
        { kind: "separator", id: "danger" },
        { kind: "item", id: "delete", label: m.deletePlugin, danger: true, onSelect: onDelete },
      ]}
    />
  );
}
