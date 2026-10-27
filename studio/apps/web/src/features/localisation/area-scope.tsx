// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AssetService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useParams, useSearch } from "@tanstack/react-router";
import { createContext, type ReactElement, type ReactNode, useContext, useMemo, useState } from "react";
import { useStore } from "zustand";
import { useApp } from "../../lib/use-app.tsx";
import type { JsonValue } from "../editor/index.ts";
import { uuidv7 } from "../plugin-canvas/screens.ts";
import { useAppDocuments } from "../plugin-canvas/use-plugin-documents.ts";
import { usePermissions } from "../security/use-security.ts";
import { localisationAreaMessages } from "./area.messages.ts";
import { type CellStatus, readStates, screensByKey } from "./grid-model.ts";
import { KEYS_PATH, localePath, readKeys, readMessages, referencedKeys, type TranslationKey } from "./l10n-model.ts";
import { type L10nSessions, L10nSessionsProvider, useL10nSessions } from "./l10n-sessions.tsx";

/** What the pages of the localisation area share (STU-111): the keys, the messages of every locale, what each plugin uses, and the permissions. */
export interface Area {
  readonly org: string;
  readonly app: string;
  readonly plugin: string | undefined;
  readonly plugins: readonly string[];
  readonly sessions: L10nSessions;
  readonly keys: readonly TranslationKey[];
  /** Messages by locale; the chosen locale's are the session's, the others the server's. */
  readonly messages: ReadonlyMap<string, Readonly<Record<string, string>>>;
  readonly states: Readonly<Record<string, CellStatus>>;
  readonly hasLocaleDoc: boolean;
  readonly usedByPlugin: ReadonlyMap<string, ReadonlySet<string>>;
  readonly screens: ReadonlyMap<string, readonly string[]>;
  readonly contextScreens: ReadonlySet<string>;
  readonly schemaVersion: JsonValue;
  readonly canEdit: boolean;
  readonly canApprove: boolean;
  readonly newId: () => string;
  /** Makes a just-added locale selectable before the app document is read back. */
  readonly rememberLocale: (locale: string) => void;
}

const Ctx = createContext<Area | null>(null);

export function useArea(): Area {
  const area = useContext(Ctx);
  if (area === null) {
    throw new Error("useArea outside AreaScope");
  }
  return area;
}

function parse(content: Uint8Array): unknown {
  try {
    return JSON.parse(new TextDecoder().decode(content));
  } catch {
    return undefined;
  }
}

function useContextScreens(app: string): ReadonlySet<string> {
  const { session } = useApp();
  const assets = useQuery({
    queryKey: ["l10n-context-assets", app],
    queryFn: async () => (await createClient(AssetService, session.transport).listAssets({ appId: app })).assets,
  });
  return useMemo(() => {
    const out = new Set<string>();
    for (const a of assets.data ?? []) {
      const m = /^l10n-context\/screen-(.+)\.png$/.exec(a.name);
      if (m?.[1] !== undefined) {
        out.add(m[1]);
      }
    }
    return out;
  }, [assets.data]);
}

interface Collected {
  readonly plugins: readonly string[];
  readonly messages: ReadonlyMap<string, Readonly<Record<string, string>>>;
  readonly usedByPlugin: ReadonlyMap<string, ReadonlySet<string>>;
  readonly screens: ReadonlyMap<string, readonly string[]>;
  readonly schemaVersion: JsonValue;
}

type Listed = { readonly pluginId: string; readonly path: string; readonly content: Uint8Array };

function usage(byPlugin: ReadonlyMap<string, { path: string; json: unknown }[]>, plugin: string | undefined) {
  const used = new Map<string, Set<string>>();
  const screens = new Map<string, readonly string[]>();
  for (const [p, list] of byPlugin) {
    used.set(p, new Set(list.flatMap((d) => referencedKeys(d.json).map((r) => r.id))));
    for (const [id, names] of plugin === undefined || plugin === p ? screensByKey(list) : []) {
      screens.set(id, [...new Set([...(screens.get(id) ?? []), ...names])]);
    }
  }
  return { used, screens };
}

/** The messages of every locale, what each plugin uses, the screens that use each key and the schema version, from the app's documents. */
function collect(docs: readonly Listed[], plugin: string | undefined, locale: string, live: unknown): Collected {
  const messages = new Map<string, Readonly<Record<string, string>>>();
  const byPlugin = new Map<string, { path: string; json: unknown }[]>();
  for (const d of docs) {
    const json = parse(d.content);
    if (d.pluginId === "" && d.path.startsWith("translations/") && d.path !== KEYS_PATH) {
      messages.set(d.path.slice("translations/".length, -".json".length), readMessages(json));
    } else if (d.pluginId !== "") {
      byPlugin.set(d.pluginId, [...(byPlugin.get(d.pluginId) ?? []), { path: d.path, json }]);
    }
  }
  const { used, screens } = usage(byPlugin, plugin);
  messages.set(locale, readMessages(live));
  const app = docs.find((d) => d.pluginId === "" && d.path === "app.json");
  const version =
    app === undefined ? undefined : (parse(app.content) as Record<string, unknown> | undefined)?.["schemaVersion"];
  return {
    plugins: [...used.keys()].sort(),
    messages,
    usedByPlugin: used,
    screens,
    schemaVersion: (version ?? 1) as JsonValue,
  };
}

function Loaded({
  sessions,
  remember,
  children,
}: {
  readonly sessions: L10nSessions;
  readonly remember: (locale: string) => void;
  readonly children: ReactNode;
}): ReactElement {
  const { app = "", org = "" } = useParams({ strict: false }) as { app?: string; org?: string };
  const plugin = (useSearch({ strict: false }) as { plugin?: string }).plugin;
  const docs = useAppDocuments(app).data;
  const can = usePermissions();
  const contextScreens = useContextScreens(app);
  const keysDoc = useStore(sessions.keys.store, (st) => st.docs[KEYS_PATH]?.content);
  const localeDoc = useStore(sessions.messages.store, (st) => st.docs[localePath(sessions.locale)]?.content);
  const value = useMemo<Area>(
    () => ({
      org,
      app,
      plugin,
      sessions,
      ...collect(docs ?? [], plugin, sessions.locale, localeDoc),
      keys: readKeys(keysDoc),
      states: readStates(localeDoc),
      hasLocaleDoc: localeDoc !== undefined,
      contextScreens,
      canEdit: can.has("translations.edit") && sessions.canEditMessages,
      canApprove: can.has("translations.approve") && sessions.canEditMessages,
      newId: uuidv7,
      rememberLocale: remember,
    }),
    [sessions, docs, plugin, org, app, keysDoc, localeDoc, can, contextScreens, remember],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

function Inner({
  remember,
  children,
}: {
  readonly remember: (locale: string) => void;
  readonly children: ReactNode;
}): ReactElement | null {
  const sessions = useL10nSessions();
  return sessions === null ? null : (
    <Loaded sessions={sessions} remember={remember}>
      {children}
    </Loaded>
  );
}

/** The sessions of the app's keys and the chosen locale's messages around the pages of the area. */
export function AreaScope({ children }: { readonly children: ReactNode }): ReactElement {
  const m = localisationAreaMessages(useLingui().i18n);
  const app = (useParams({ strict: false }) as { app?: string }).app ?? "";
  const docs = useAppDocuments(app);
  const [added, setAdded] = useState<readonly string[]>([]);
  if (docs.data === undefined) {
    return (
      <p role="status" className="text-body text-secondary">
        {m.loading}
      </p>
    );
  }
  return (
    <L10nSessionsProvider appId={app} documents={docs.data} addedLocales={added}>
      <Inner remember={(l) => setAdded((cur) => [...cur, l])}>{children}</Inner>
    </L10nSessionsProvider>
  );
}
