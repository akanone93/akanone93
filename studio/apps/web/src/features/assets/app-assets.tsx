// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppService, AssetService } from "@plux/api";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactElement, useMemo } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { useLock } from "../editor/editor-provider.tsx";
import { assetsMessages } from "../panels/assets.messages.ts";
import { AssetsTab } from "../panels/assets-tab.tsx";
import { underAppLock } from "../plugin-canvas/use-assets-tab.ts";
import { useAppDocuments, type usePluginDocuments } from "../plugin-canvas/use-plugin-documents.ts";

/**
 * The app's assets at the app level (A3): the same Assets panel as in a plugin, its uploads and deletes made under the
 * app's lock, which the surrounding editor holds (the app canvas's, or the Assets page's). Uses are counted across the app's documents and its icon.
 */
export function AppAssets({
  appId,
  documents,
}: {
  readonly appId: string;
  readonly documents: ReturnType<typeof usePluginDocuments>["data"];
}): ReactElement {
  const { i18n } = useLingui();
  const { session } = useApp();
  const lock = useLock();
  const queries = useQueryClient();
  const appRecord = useQuery({
    queryKey: ["app", appId],
    enabled: appId !== "",
    queryFn: async () => (await createClient(AppService, session.transport).getApp({ id: appId })).app,
  });
  // Every plugin's documents, as the plugin canvas reads them: a use in any of them is a use.
  const project = useAppDocuments(appId);
  const client = useMemo(
    () => underAppLock(createClient(AssetService, session.transport), () => lock),
    [session.transport, lock],
  );
  const elsewhere = useMemo(() => {
    const decoder = new TextDecoder();
    const docs = [...(project.data ?? []), ...(documents ?? [])]
      .filter((d) => d.path !== "assets/index.json")
      .map((d) => ({ path: d.path, text: decoder.decode(d.content) }));
    const icon = appRecord.data?.iconAssetId ?? "";
    const label = assetsMessages(i18n).appIcon;
    return (id: string): readonly string[] => [
      ...(icon === id ? [label] : []),
      ...[...new Set(docs.filter((d) => d.text.includes(id)).map((d) => d.path))],
    ];
  }, [project.data, documents, appRecord.data, i18n]);
  return (
    <AssetsTab
      client={client}
      appId={appId}
      session=""
      elsewhere={elsewhere}
      onChanged={() => void queries.invalidateQueries({ queryKey: ["assets", appId] })}
    />
  );
}
