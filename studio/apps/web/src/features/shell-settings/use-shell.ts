// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { MessageInitShape } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { AppProjectService, type ShellSettingsSchema } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useApp } from "../../lib/use-app.tsx";

const key = (appId: string) => ["shell-status", appId] as const;

/** The app's shell settings compared with those its project was generated from (GEN-004). */
export function useShellStatus(appId: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: key(appId),
    queryFn: () => createClient(AppProjectService, session.transport).getShellStatus({ appId }),
  });
}

/** Stores the shell settings and refreshes the comparison. */
export function useSaveShell(appId: string) {
  const { session } = useApp();
  return async (settings: MessageInitShape<typeof ShellSettingsSchema>): Promise<void> => {
    await createClient(AppProjectService, session.transport).setShellSettings({ appId, settings });
    await session.queryClient.invalidateQueries({ queryKey: key(appId) });
  };
}
