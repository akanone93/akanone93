// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { PluginSummary } from "@plux/api";
import { Icon } from "@plux/ui";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@plux/ui/components/input-group";
import { type ReactElement, type ReactNode, useState } from "react";
import { DockTabs, type Pane, useStoredFlag } from "../panels/dock-tabs.tsx";
import type { appCanvasMessages } from "./app-canvas.messages.ts";
import type { Frame } from "./app-canvas.model.ts";
import { hueOf, initialOf, type SceneBox } from "./app-canvas.scene.ts";
import { Panel } from "./canvas-chrome.tsx";

type Messages = ReturnType<typeof appCanvasMessages>;
type Filter = "all" | "draft" | "errors" | "archived";
const filters: readonly Filter[] = ["all", "draft", "errors", "archived"];

/** Whether a plugin passes a status filter of the Plugins tab. */
export function passes(filter: Filter, box: SceneBox, s: PluginSummary | undefined): boolean {
  switch (filter) {
    case "draft":
      return s !== undefined && (s.unpublishedChanges || Number(s.versions) === 0);
    case "errors":
      return s !== undefined && s.validationErrors > 0n;
    case "archived":
      return box.archived;
    default:
      return true;
  }
}

/** The state dot of a row: the worst badge of the box. */
function stateOf(b: SceneBox): string {
  if (b.archived) {
    return "archived";
  }
  for (const status of ["danger", "warning"] as const) {
    if (b.badges.some((x) => x.status === status)) {
      return status;
    }
  }
  return "ok";
}

/**
 * The app canvas's left dock, as the plugin editor's: the bold rail (Plugins, Frames, Overview) and the header (which
 * canvas, which app, how big). Plugins lists every plugin with its mark, key and owner, screens and state, searchable and
 * filtered by status; a row selects its plugin and brings it into view, a double click opens it. The dock folds to its
 * rail.
 */
export function AppDock({
  appName,
  boxes,
  frames,
  selected,
  summaries,
  entryKey,
  messages: m,
  onShow,
  onOpen,
  frameTools,
}: {
  readonly appName: string | undefined;
  readonly boxes: readonly SceneBox[];
  readonly frames: readonly Frame[];
  readonly selected: ReadonlySet<string>;
  readonly summaries: ReadonlyMap<string, PluginSummary>;
  readonly entryKey: string | undefined;
  readonly messages: Messages;
  readonly onShow: (box: SceneBox, additive: boolean) => void;
  readonly onOpen: (box: SceneBox) => void;
  readonly frameTools: ReactNode;
}): ReactElement {
  const [folded, setFolded] = useStoredFlag("plux.dock.app.folded", false);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const plugins = boxes.filter((b) => !b.native);
  const q = query.trim().toLowerCase();
  const shown = [...boxes]
    .filter((b) => passes(filter, b, summaries.get(b.id)))
    .filter((b) => q === "" || `${b.title} ${b.key}`.toLowerCase().includes(q))
    .sort((a, b) => a.title.localeCompare(b.title));
  const count = (f: Filter) => plugins.filter((b) => passes(f, b, summaries.get(b.id))).length;
  const screens = plugins.reduce((n, b) => n + (b.screens ?? 0), 0);
  const entry = plugins.find((b) => b.key === entryKey);
  const figures: readonly { key: Parameters<Messages["overviewFigure"]>[0]; value: number; tone?: string }[] = [
    { key: "plugins", value: plugins.length },
    { key: "screens", value: screens },
    { key: "frames", value: frames.length },
    { key: "draft", value: count("draft") },
    { key: "errors", value: count("errors"), ...(count("errors") > 0 ? { tone: "error" } : {}) },
    { key: "archived", value: count("archived") },
  ];
  const panes: readonly Pane[] = [
    {
      value: "plugins",
      label: m.layers,
      icon: "puzzle-piece",
      meta: String(plugins.length),
      toolbar: (
        <div className="flex flex-col gap-2">
          <InputGroup>
            <InputGroupAddon>
              <Icon name="magnifying-glass" />
            </InputGroupAddon>
            <InputGroupInput
              aria-label={m.searchPlugins}
              placeholder={m.searchPlugins}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </InputGroup>
          {/* biome-ignore lint/a11y/useSemanticElements: a labelled group of filter toggles, not a form's fieldset */}
          <div role="group" aria-label={m.pluginFilters} className="gallery-kinds">
            {filters.map((f) => (
              <button
                key={f}
                type="button"
                className="gallery-kind"
                aria-pressed={filter === f}
                onClick={() => setFilter(f)}
              >
                {m.pluginFilter(f)}
                <span className="tabular-nums opacity-70">{f === "all" ? plugins.length : count(f)}</span>
              </button>
            ))}
          </div>
        </div>
      ),
      panel: (
        <>
          {shown.length === 0 && <p className="dock-note">{m.noPluginMatches}</p>}
          {/* A list box: its options are the plugins, selected as on the canvas; arrows move, Enter or Space selects. */}
          <div
            role="listbox"
            aria-label={m.layers}
            aria-multiselectable="true"
            className="flex flex-col gap-1"
            onKeyDown={(e) => {
              if (e.key !== "ArrowDown" && e.key !== "ArrowUp") {
                return;
              }
              e.preventDefault();
              const options = [...e.currentTarget.querySelectorAll<HTMLElement>("[role=option]")];
              const at = options.indexOf(document.activeElement as HTMLElement);
              options[Math.max(0, Math.min(options.length - 1, at + (e.key === "ArrowDown" ? 1 : -1)))]?.focus();
            }}
          >
            {shown.map((b) => (
              <div
                key={b.id}
                role="option"
                tabIndex={0}
                aria-label={m.showOnCanvas(b.title)}
                aria-selected={selected.has(b.id)}
                data-native={b.native ? "" : undefined}
                className="canvas-layer app-dock-row"
                onClick={(e) => onShow(b, e.shiftKey || e.metaKey || e.ctrlKey)}
                onDoubleClick={() => onOpen(b)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    onShow(b, e.shiftKey);
                  }
                }}
              >
                <span className="app-dock-mark" data-hue={b.native || b.archived ? undefined : hueOf(b.key, 5) + 1}>
                  {initialOf(b.title)}
                </span>
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate text-sm font-medium">{b.title}</span>
                  <span className="truncate text-xs text-muted-foreground">
                    {b.native ? m.native : (b.subtitle ?? b.key)}
                  </span>
                </span>
                {b.release !== undefined && <span className="app-dock-chip">{b.release}</span>}
                <span className="canvas-layer-dot" data-state={stateOf(b)} aria-hidden="true" />
              </div>
            ))}
          </div>
        </>
      ),
    },
    {
      value: "frames",
      label: m.framesTitle,
      icon: "frame",
      meta: String(frames.length),
      panel:
        frames.length === 0 ? (
          <p className="dock-note">{m.noFrames}</p>
        ) : (
          <div className="canvas-frames flex flex-col gap-0.5">{frameTools}</div>
        ),
    },
    {
      value: "overview",
      label: m.overviewTab,
      icon: "gauge",
      panel: (
        <div className="flex flex-col gap-4">
          <dl className="overview-figures">
            {figures.map((f) => (
              <div key={f.key} className="overview-figure" data-tone={f.tone}>
                <dt>{m.overviewFigure(f.key)}</dt>
                <dd>{f.value}</dd>
              </div>
            ))}
          </dl>
          <div className="overview-screen">
            <Icon name="house" size="pill" />
            <span className="min-w-0 flex-1 truncate">{m.entryPlugin}</span>
            <strong className="truncate">{entry?.title ?? m.noEntry}</strong>
          </div>
        </div>
      ),
    },
  ];
  return (
    <Panel className="canvas-layers canvas-side app-dock flex flex-col self-stretch" data-folded={folded || undefined}>
      <DockTabs
        panes={panes}
        defaultValue="plugins"
        mark="squares-four"
        folded={folded}
        onFold={setFolded}
        foldLabel={m.hidePanel}
        head={
          <div className="flex min-w-0 flex-col">
            <span className="dock-head-kicker">{m.dockTitle}</span>
            <h2 className="dock-head-title">{appName ?? ""}</h2>
            <span className="dock-head-meta">{m.dockSummary(plugins.length, frames.length)}</span>
          </div>
        }
      />
    </Panel>
  );
}
