// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { PluginService } from "@plux/api";
import { Button } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import { type ReactElement, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { appsMessages } from "./apps.messages.ts";
import { LimitsTable } from "./limits-usage.tsx";

/**
 * What one plugin of the app uses of the limits that apply to a plugin (LIM-005): the plugins as buttons, the table of
 * the chosen one below. The server answers a plugin of another organisation as not found, which shows as a failure.
 */
export function PluginUsage({ appId }: { readonly appId: string }): ReactElement {
  const m = appsMessages(useLingui().i18n);
  const { session } = useApp();
  const [chosen, setChosen] = useState<string | undefined>(undefined);
  const plugins = useQuery({
    queryKey: ["app-plugins-usage", appId],
    queryFn: async () => (await createClient(PluginService, session.transport).listPlugins({ appId })).plugins,
  });
  const current = chosen ?? plugins.data?.[0]?.id;
  const limits = useQuery({
    queryKey: ["plugin-limits", current],
    enabled: current !== undefined,
    queryFn: async () =>
      (await createClient(PluginService, session.transport).listPluginLimits({ pluginId: current ?? "" })).limits,
  });
  return (
    <section aria-labelledby="app-settings-plugin-usage" className="flex flex-col gap-3">
      <h2 id="app-settings-plugin-usage" className="text-section">
        {m.pluginUsageTitle}
      </h2>
      <p className="text-body text-secondary">{m.pluginUsageCaption}</p>
      {plugins.isSuccess && plugins.data.length === 0 && (
        <p className="text-body text-secondary">{m.pluginUsageNone}</p>
      )}
      {plugins.isSuccess && plugins.data.length > 0 && (
        <fieldset className="flex flex-wrap gap-2">
          <legend className="sr-only">{m.pluginPicker}</legend>
          {plugins.data.map((p) => (
            <Button
              key={p.id}
              size="sm"
              variant={p.id === current ? "primary" : "secondary"}
              aria-pressed={p.id === current}
              onClick={() => setChosen(p.id)}
            >
              {p.name === "" ? p.key : p.name}
            </Button>
          ))}
        </fieldset>
      )}
      {(plugins.isError || limits.isError) && (
        <p role="alert" className="text-body text-danger">
          {m.pluginUsageLoadFailed}
        </p>
      )}
      {limits.isSuccess && <LimitsTable limits={limits.data} />}
    </section>
  );
}
