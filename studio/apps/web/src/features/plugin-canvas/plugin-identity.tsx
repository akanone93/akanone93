// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppService } from "@plux/api";
import { Icon } from "@plux/ui";
import { Separator } from "@plux/ui/components/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@plux/ui/components/tooltip";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { Panel } from "../app-canvas/canvas-chrome.tsx";
import { CardVisual } from "../apps/card-visual.tsx";
import { pluginCanvasMessages } from "./plugin-canvas.messages.ts";

/**
 * The start of the plugin canvas's top bar: the way back to the app canvas, the app's mark, and the app over the
 * plugin's name — the page's heading.
 */
export function PluginIdentity({
  org,
  appId,
  plugin,
}: {
  readonly org: string | undefined;
  readonly appId: string | undefined;
  readonly plugin: string | undefined;
}): ReactElement {
  const id = appId ?? "";
  const m = pluginCanvasMessages(useLingui().i18n);
  const { session } = useApp();
  // The same cache entry as the app canvas's header, so going back and forth costs no request.
  const app = useQuery({
    queryKey: ["app", id],
    queryFn: async () => (await createClient(AppService, session.transport).getApp({ id })).app,
    enabled: id !== "",
  });
  const appName = app.data?.name ?? "";
  return (
    <Panel className="canvas-bar flex min-w-0 items-center gap-2 p-1.5 pe-4">
      <Tooltip>
        <TooltipTrigger
          render={
            <Link
              to="/o/$org/apps/$app"
              params={{ org: org ?? "", app: id }}
              aria-label={m.backToApp(appName)}
              className="inline-flex size-8 shrink-0 items-center justify-center rounded-lg hover:bg-muted"
            />
          }
        >
          <Icon name="arrow-left" />
        </TooltipTrigger>
        <TooltipContent side="bottom">{m.backToApp(appName)}</TooltipContent>
      </Tooltip>
      <Separator orientation="vertical" className="h-6" />
      {app.data !== undefined && <CardVisual app={app.data} compact />}
      <div className="flex min-w-0 flex-col">
        <span className="truncate text-xs text-muted-foreground">{appName}</span>
        <h1 className="truncate text-sm font-semibold">{plugin ?? ""}</h1>
      </div>
    </Panel>
  );
}
