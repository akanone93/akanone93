// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppService, AssetService, type Document } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { KEYS_PATH } from "../localisation/l10n-model.ts";
import { APP_DESIGNER_PATHS, useAppSession } from "../localisation/l10n-sessions.tsx";
import { assetsMessages } from "../panels/assets.messages.ts";
import type { FixContext } from "./problem-fixes.tsx";

/**
 * The names of the app's stored asset files, for the validator (it called every listed asset missing without them); an
 * empty list when the listing fails, so validation still runs. Undefined while it loads.
 */
export function useAssetNames(app: string | undefined): readonly string[] | undefined {
  const appId = app ?? "";
  const { session } = useApp();
  const stored = useQuery({
    queryKey: ["assets", appId],
    enabled: appId !== "",
    queryFn: async () => (await createClient(AssetService, session.transport).listAssets({ appId })).assets,
  });
  return useMemo(() => (stored.isError ? [] : stored.data?.map((a) => a.name)), [stored.isError, stored.data]);
}

/** Where an asset is used outside one plugin: the app's icon, and every other document that names it. */
export function useAssetElsewhere(
  app: string | undefined,
  plugin: string | undefined,
  documents: readonly Document[] | undefined,
): (assetId: string) => readonly string[] {
  const appId = app ?? "";
  const pluginId = plugin ?? "";
  const { session } = useApp();
  const { i18n } = useLingui();
  const record = useQuery({
    queryKey: ["app", appId],
    enabled: appId !== "",
    queryFn: async () => (await createClient(AppService, session.transport).getApp({ id: appId })).app,
  });
  return useMemo(() => {
    const decoder = new TextDecoder();
    const others = (documents ?? [])
      .filter((d) => d.pluginId !== pluginId && d.path !== "assets/index.json")
      .map((d) => ({ path: d.path, text: decoder.decode(d.content) }));
    const icon = record.data?.iconAssetId ?? "";
    const label = assetsMessages(i18n).appIcon;
    return (id: string) => [
      ...(icon === id ? [label] : []),
      ...others.filter((d) => d.text.includes(id)).map((d) => d.path),
    ];
  }, [documents, pluginId, record.data, i18n]);
}

/** The app documents the plugin canvas's JSON editor can open: those its app session holds. */
const APP_JSON_PATHS: ReadonlySet<string> = new Set([...APP_DESIGNER_PATHS, KEYS_PATH]);

/**
 * What the Problems tab's fixes reach (P1): the JSON editor, opened on a finding's file, and the app session. The path is
 * the one the editor opens on, until it closes.
 */
export function useProblemFixes(
  documents: readonly { readonly path: string }[] | undefined,
  showJson: () => void,
): {
  readonly fixes: Omit<FixContext, "assets">;
  readonly jsonPath: string | undefined;
  readonly clearJsonPath: () => void;
} {
  const [jsonPath, setJsonPath] = useState<string | undefined>();
  const appSession = useAppSession();
  const fixes = useMemo(() => {
    const own = new Set((documents ?? []).map((d) => d.path));
    return {
      editable: (file: string) => own.has(file) || APP_JSON_PATHS.has(file),
      openJson: (file: string) => {
        setJsonPath(file);
        showJson();
      },
      app: appSession === null ? null : { session: appSession.session, canEdit: appSession.canEdit },
    };
  }, [documents, showJson, appSession]);
  return { fixes, jsonPath, clearJsonPath: () => setJsonPath(undefined) };
}
