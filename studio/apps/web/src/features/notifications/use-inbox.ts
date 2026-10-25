// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { NotificationService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useApp } from "../../lib/use-app.tsx";

/** ListInbox through the shared query cache; the events stream invalidates the "inbox" key. */
export function useInbox(options: { readonly unreadOnly: boolean; readonly limit: number }) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["inbox", options.unreadOnly, options.limit],
    queryFn: () => createClient(NotificationService, session.transport).listInbox(options),
  });
}

export function useMarkRead() {
  const { session } = useApp();
  return async (ids: readonly string[]) => {
    await createClient(NotificationService, session.transport).markRead({ ids: [...ids] });
    await session.queryClient.invalidateQueries({ queryKey: ["inbox"] });
  };
}
