// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { NativeCatalogueService } from "@plux/api";
import type { NativeCatalogueDocument, WidgetDescriptor } from "@plux/schema";
import { useQuery } from "@tanstack/react-query";
import { useApp } from "../../lib/use-app.tsx";

const NONE: readonly WidgetDescriptor[] = [];

/** Palette entries for the native slots of a catalogue (WGT-030): a node whose `type` is the slot name. */
export function nativeDescriptors(doc: NativeCatalogueDocument): readonly WidgetDescriptor[] {
  return (doc.slots ?? []).map((s, i) => ({
    type: s.type,
    id: i,
    layer: 0,
    phase: "native",
    revision: 1,
    runtimes: [],
    category: "native",
    icon: "extension",
    description: s.description ?? "",
    platforms: [],
    cost: 0,
    accessibility: { role: "none", interactive: false },
    typeParameters: [],
    props: (s.props ?? []).map((p, n) => ({
      name: p.name,
      id: n + 1,
      type: p.type,
      required: p.required === true,
      ...(p.default === undefined ? {} : { default: p.default }),
      revision: 1,
      bindable: true,
      ...(p.description === undefined ? {} : { description: p.description }),
    })),
    events: [],
    slots: [],
  })) as unknown as readonly WidgetDescriptor[];
}

/** The native route names of the app's newest host build (NAV-002), cached per app; empty while loading. */
export function useNativeRoutes(appId: string): readonly string[] {
  const { session } = useApp();
  const q = useQuery({
    queryKey: ["native-routes", appId],
    enabled: appId !== "",
    queryFn: async () => {
      const client = createClient(NativeCatalogueService, session.transport);
      const newest = (await client["listHostBuilds"]({ appId })).hostBuilds[0];
      if (newest === undefined) {
        return [];
      }
      const res = await client["getNativeCatalogue"]({ appId, hostBuild: newest.hostBuild });
      const doc = JSON.parse(new TextDecoder().decode(res.catalogue)) as NativeCatalogueDocument;
      return (doc.routes ?? []).map((r) => r.name);
    },
  });
  return q.data ?? [];
}

/**
 * The native widgets of the app's newest host build, fetched with GetNativeCatalogue and cached per app;
 * empty while loading and when the app has no build.
 */
export function useNativeWidgets(appId: string): readonly WidgetDescriptor[] {
  const { session } = useApp();
  const q = useQuery({
    queryKey: ["native-widgets", appId],
    enabled: appId !== "",
    queryFn: async () => {
      const client = createClient(NativeCatalogueService, session.transport);
      const newest = (await client["listHostBuilds"]({ appId })).hostBuilds[0];
      if (newest === undefined) {
        return NONE;
      }
      const res = await client["getNativeCatalogue"]({ appId, hostBuild: newest.hostBuild });
      return nativeDescriptors(JSON.parse(new TextDecoder().decode(res.catalogue)) as NativeCatalogueDocument);
    },
  });
  return q.data ?? NONE;
}
