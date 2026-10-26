// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { NativeCatalogueService } from "@plux/api";
import type { NativeCatalogueDocument } from "@plux/schema";
import { useQuery } from "@tanstack/react-query";
import { useApp } from "../../lib/use-app.tsx";
import type { CustomAction } from "./graph-model.ts";

const NONE: readonly CustomAction[] = [];

/** The custom actions of the app's newest host build (ACT-060), cached per app; empty while loading and when there is no build. */
export function useNativeActions(appId: string): readonly CustomAction[] {
  const { session } = useApp();
  const q = useQuery({
    queryKey: ["native-actions", appId],
    enabled: appId !== "",
    queryFn: async () => {
      const client = createClient(NativeCatalogueService, session.transport);
      const newest = (await client["listHostBuilds"]({ appId })).hostBuilds[0];
      if (newest === undefined) {
        return NONE;
      }
      const res = await client["getNativeCatalogue"]({ appId, hostBuild: newest.hostBuild });
      const doc = JSON.parse(new TextDecoder().decode(res.catalogue)) as NativeCatalogueDocument;
      return (doc.actions ?? []).map((a) => ({
        name: a.name,
        description: a.description,
        inputs: (a.inputs ?? []).map((i) => ({ name: i.name, type: i.type, required: i.required === true })),
        output: a.output,
      }));
    },
  });
  return q.data ?? NONE;
}
