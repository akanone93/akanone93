// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { ComponentService, type LibraryItem, TemplateService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { loadUser } from "../../lib/guards.ts";
import { useApp } from "../../lib/use-app.tsx";
import type { LibrarySearch } from "./library.schema.ts";
import { filterItems } from "./library-model.ts";

export type LibraryKind = "component" | "template";

/** Whether the signed-in person holds `permission`; false while it loads (the server checks it too). */
export function useCan(permission: string): boolean {
  const { session } = useApp();
  const user = useQuery({ queryKey: ["current-user"], queryFn: () => loadUser(session) });
  return user.data?.permissions.includes(permission) ?? false;
}

/** O4: the components and templates the caller may see, narrowed by the server filters then the client ones. */
export function useLibraryItems(search: LibrarySearch) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["library", search.kind ?? "", search.visibility ?? "", search.q ?? ""],
    queryFn: async (): Promise<LibraryItem[]> => {
      const request = { visibility: search.visibility ?? "", query: search.q ?? "", page: { pageSize: 200 } };
      const [components, templates] = await Promise.all([
        search.kind === "template"
          ? undefined
          : createClient(ComponentService, session.transport).listLibraryComponents(request),
        search.kind === "component"
          ? undefined
          : createClient(TemplateService, session.transport).listLibraryTemplates(request),
      ]);
      return [...(components?.items ?? []), ...(templates?.items ?? [])];
    },
    select: (items) => filterItems(items, search),
  });
}

export function useLibraryClients() {
  const { session } = useApp();
  return {
    components: createClient(ComponentService, session.transport),
    templates: createClient(TemplateService, session.transport),
    invalidate: () => session.queryClient.invalidateQueries({ queryKey: ["library"] }),
  };
}
