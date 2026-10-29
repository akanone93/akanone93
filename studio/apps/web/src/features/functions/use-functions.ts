// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { AppService, FunctionAdminService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useApp } from "../../lib/use-app.tsx";
import { metricsWindowSeconds } from "./functions.schema.ts";

function useAdmin() {
  const { session } = useApp();
  return createClient(FunctionAdminService, session.transport);
}

export function useFunctions(app: string, environmentId: string | undefined) {
  const admin = useAdmin();
  return useQuery({
    queryKey: ["functions", app, environmentId],
    enabled: environmentId !== undefined,
    queryFn: () => admin.listFunctions({ appId: app, environmentId: environmentId ?? "" }),
  });
}

export function useFunction(app: string, environmentId: string | undefined, name: string) {
  const admin = useAdmin();
  return useQuery({
    queryKey: ["function", app, environmentId, name],
    enabled: environmentId !== undefined,
    queryFn: () => admin.getFunction({ appId: app, environmentId: environmentId ?? "", name }),
  });
}

export function useVersions(app: string, environmentId: string | undefined, name: string) {
  const admin = useAdmin();
  return useQuery({
    queryKey: ["function-versions", app, environmentId, name],
    enabled: environmentId !== undefined,
    queryFn: () => admin.listVersions({ appId: app, environmentId: environmentId ?? "", name }),
  });
}

export function useMetrics(app: string, environmentId: string | undefined, name: string) {
  const admin = useAdmin();
  return useQuery({
    queryKey: ["function-metrics", app, environmentId, name],
    enabled: environmentId !== undefined,
    queryFn: () =>
      admin.getMetrics({ appId: app, environmentId: environmentId ?? "", name, windowSeconds: metricsWindowSeconds }),
  });
}

export function useLogs(app: string, environmentId: string | undefined, name: string) {
  const admin = useAdmin();
  return useQuery({
    queryKey: ["function-logs", app, environmentId, name],
    enabled: environmentId !== undefined,
    queryFn: () => admin.listLogs({ appId: app, environmentId: environmentId ?? "", name, version: 0 }),
  });
}

/** Points an alias at a version (FN-006) and refreshes what shows it. */
export function useSetAlias(app: string, environmentId: string | undefined, name: string) {
  const admin = useAdmin();
  const { session } = useApp();
  return async (alias: string, version: number) => {
    await admin.setAlias({ appId: app, environmentId: environmentId ?? "", name, alias: { name: alias, version } });
    await session.queryClient.invalidateQueries({ queryKey: ["function", app] });
  };
}

/** The environments of the app: what the logs tab offers to look at. */
export function useEnvironments(app: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["environments", app],
    queryFn: () => createClient(AppService, session.transport).listEnvironments({ appId: app }),
  });
}
