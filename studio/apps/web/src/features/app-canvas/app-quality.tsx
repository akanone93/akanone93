// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { Document } from "@plux/api";
import { Icon } from "@plux/ui";
import { Button } from "@plux/ui/components/button";
import { type ReactNode, useMemo } from "react";
import type { LoadedDoc, PageInfo } from "../safety/index.ts";
import { appCanvasMessages } from "./app-canvas.messages.ts";
import { appQuality, type PluginQuality } from "./app-quality.ts";

/** The Quality tab of the app canvas's dock (#288): per plugin, the screens to look at, each opening its screen. */
export function AppQuality(props: {
  readonly plugins: readonly PluginQuality[];
  readonly pluginName: (id: string) => string;
  readonly onScreen: (pluginId: string, pageId: string) => void;
}): ReactNode {
  const { i18n } = useLingui();
  const m = appCanvasMessages(i18n);
  return (
    <div className="flex flex-col gap-3">
      {props.plugins.length === 0 ? (
        <p className="dock-note">{m.qualityNone}</p>
      ) : (
        props.plugins.map((p) => (
          <section key={p.pluginId} aria-label={props.pluginName(p.pluginId)} className="flex flex-col gap-1">
            <h3 className="text-label font-medium">{props.pluginName(p.pluginId)}</h3>
            <ul className="flex flex-col gap-1">
              {p.screens.map((s) => (
                <li key={s.pageId}>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-auto w-full flex-col items-start gap-0.5 px-1 py-1 text-start"
                    onClick={() => props.onScreen(p.pluginId, s.pageId)}
                  >
                    <span className="truncate font-medium">{s.title}</span>
                    {s.lowContrast > 0 && (
                      <span className="flex items-center gap-1 text-caption text-muted-foreground">
                        <Icon name="warning" size="pill" />
                        {m.qualityContrast(s.lowContrast, s.modes.join(" · "))}
                      </span>
                    )}
                    {s.status !== "within" && (
                      <span
                        className="flex items-center gap-1 text-caption"
                        data-tone={s.status === "over" ? "error" : "warning"}
                      >
                        <Icon name={s.status === "over" ? "x-circle" : "warning"} size="pill" />
                        {m.qualityNodes(s.nodes, s.limit)}
                      </span>
                    )}
                  </Button>
                </li>
              ))}
            </ul>
          </section>
        ))
      )}
      <p className="text-caption text-muted-foreground">{m.qualityNote}</p>
    </div>
  );
}

/** The Quality pane of the app canvas's dock: its label, how many screens need a look, and the list. */
export function useQualityPane(
  project: readonly Document[] | undefined,
  loaded: readonly LoadedDoc[],
  pages: ReadonlyMap<string, PageInfo>,
  pluginName: (id: string) => string,
  onScreen: (pluginId: string, pageId: string) => void,
): { readonly label: string; readonly meta: string; readonly panel: ReactNode } {
  const { i18n } = useLingui();
  const plugins = useMemo(() => {
    const byPath = new Map(loaded.map((d) => [d.path, d.json]));
    return appQuality(project ?? [], pages.values(), (p) => {
      const json = byPath.get(p.path);
      return typeof json === "object" && json !== null ? (json as Record<string, unknown>)["root"] : undefined;
    });
  }, [project, loaded, pages]);
  const count = plugins.reduce((n, p) => n + p.screens.length, 0);
  return {
    label: appCanvasMessages(i18n).qualityTab,
    meta: String(count),
    panel: <AppQuality plugins={plugins} pluginName={pluginName} onScreen={onScreen} />,
  };
}
