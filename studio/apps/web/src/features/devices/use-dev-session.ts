// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { AppService, DevSessionKind, DevSessionService, DevThemeMode, type Environment } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { useApp } from "../../lib/use-app.tsx";

export const themes = {
  light: DevThemeMode.LIGHT,
  dark: DevThemeMode.DARK,
  system: DevThemeMode.SYSTEM,
} as const;

/** The dev session client of the signed-in session. */
export function useDevClient() {
  const { session } = useApp();
  return useMemo(() => createClient(DevSessionService, session.transport), [session.transport]);
}

/** The environment dev sessions open in by default (the server's `devSession.enabledEnvironments`). */
export const DEV_ENVIRONMENT = "development";

/**
 * The environment a Studio dev session opens in: the app's `development` environment, else its first one that is
 * not production. The server refuses a session without one, and in a production environment.
 */
export function devEnvironment(environments: readonly Environment[]): Environment | undefined {
  const open = environments.filter((e) => !e.production);
  return open.find((e) => e.key === DEV_ENVIRONMENT) ?? open[0];
}

/**
 * The Studio dev session of an app (contract #17): created once, when `enabled`, in the app's development
 * environment, and shared by the pairing dialog, the send dialog, the controls and the drawer. The server follows
 * the stored draft; Studio never pushes. A refused session is not asked for again until the editor opens anew.
 */
export function useStudioDevSession(appId: string, enabled: boolean): string | undefined {
  const client = useDevClient();
  const { session } = useApp();
  const apps = useMemo(() => createClient(AppService, session.transport), [session.transport]);
  const created = useQuery({
    queryKey: ["studio-dev-session", appId],
    enabled: enabled && appId !== "",
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    queryFn: async () => {
      const env = devEnvironment((await apps.listEnvironments({ appId })).environments);
      if (env === undefined) {
        return "";
      }
      return (
        (await client.createSession({ appId, environmentId: env.id, kind: DevSessionKind.STUDIO })).session?.id ?? ""
      );
    },
  });
  return created.data === "" ? undefined : created.data;
}
