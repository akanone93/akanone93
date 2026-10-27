// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { DevSessionKind, DevSessionService, DevThemeMode } from "@plux/api";
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

/**
 * The Studio dev session of an app (contract #17): created once, when `enabled`, and shared by the pairing
 * dialog, the send dialog, the controls and the drawer. The server follows the stored draft; Studio never pushes.
 */
export function useStudioDevSession(appId: string, enabled: boolean): string | undefined {
  const client = useDevClient();
  const created = useQuery({
    queryKey: ["studio-dev-session", appId],
    enabled: enabled && appId !== "",
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
    queryFn: async () => (await client.createSession({ appId, kind: DevSessionKind.STUDIO })).session?.id ?? "",
  });
  return created.data === "" ? undefined : created.data;
}
