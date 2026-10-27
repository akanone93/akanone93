// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Document } from "@plux/api";
import { createContext, type ReactElement, type ReactNode, useContext, useEffect, useMemo, useState } from "react";
import {
  EditorProvider,
  type EditSession,
  type LockController,
  useCanEdit,
  useEditSession,
  useLock,
} from "../editor/index.ts";
import { KEYS_PATH, localePath, localeScope, readLocales } from "./l10n-model.ts";

/** What the localisation features of the plugin canvas edit: the app's keys and one locale's messages. */
export interface L10nSessions {
  readonly locales: readonly string[];
  readonly defaultLocale: string;
  /** The locale whose messages the session below holds; changing it takes that locale's lock (I18N-008). */
  readonly locale: string;
  readonly setLocale: (locale: string) => void;
  readonly keys: EditSession;
  readonly canEditKeys: boolean;
  readonly messages: EditSession;
  readonly canEditMessages: boolean;
}

const Context = createContext<L10nSessions | null>(null);

/** The app's own draft session, which holds the app lock: the app-level documents the designers edit from the plugin canvas (the theme). */
export interface AppSession {
  readonly session: EditSession;
  readonly canEdit: boolean;
  readonly lock: LockController;
}

const AppContext = createContext<AppSession | null>(null);

/** The surrounding app session, or null until it is ready. */
export function useAppSession(): AppSession | null {
  return useContext(AppContext);
}

/** The app-level documents besides the translation keys that the plugin canvas edits under the app lock. */
export const APP_DESIGNER_PATHS: readonly string[] = ["theme.json", "app.json"];

/** The surrounding sessions, or null where the app's documents are not loaded. */
export function useL10nSessions(): L10nSessions | null {
  return useContext(Context);
}

function Tap({ onSlot }: { readonly onSlot: (slot: AppSession) => void }): null {
  const session = useEditSession();
  const canEdit = useCanEdit();
  const lock = useLock();
  useEffect(() => onSlot({ session, canEdit, lock }), [session, canEdit, lock, onSlot]);
  return null;
}

function parse(d: Document | undefined): unknown {
  try {
    return d === undefined ? undefined : JSON.parse(new TextDecoder().decode(d.content));
  } catch {
    return undefined;
  }
}

/**
 * Gives its children, which keep the plugin's own editor, the sessions that edit the app's translation keys (the draft lock) and one locale's messages
 * (that locale's lock), beside the plugin's own session. Messages never share a lock with the keys, so a translator
 * holds a locale without holding the draft (I18N-008).
 */
export function L10nSessionsProvider(props: {
  readonly appId: string;
  readonly documents: readonly Document[] | undefined;
  readonly children: ReactNode;
}): ReactElement {
  const { appId, documents } = props;
  const app = useMemo(() => readLocales(parse(documents?.find((d) => d.path === "app.json"))), [documents]);
  const [chosen, setLocale] = useState<string | undefined>(undefined);
  const locale = chosen !== undefined && app?.supported.includes(chosen) ? chosen : (app?.defaultLocale ?? "");
  const keyDocs = useMemo(
    () =>
      (documents ?? []).filter(
        (d) => d.pluginId === "" && (d.path === KEYS_PATH || APP_DESIGNER_PATHS.includes(d.path)),
      ),
    [documents],
  );
  const messageDocs = useMemo(
    () => (documents ?? []).filter((d) => d.pluginId === "" && d.path === localePath(locale)),
    [documents, locale],
  );
  const [keys, setKeys] = useState<AppSession | null>(null);
  const [messages, setMessages] = useState<AppSession | null>(null);
  const value = useMemo<L10nSessions | null>(
    () =>
      app === undefined || keys === null || messages === null
        ? null
        : {
            locales: app.supported,
            defaultLocale: app.defaultLocale,
            locale,
            setLocale,
            keys: keys.session,
            canEditKeys: keys.canEdit,
            messages: messages.session,
            canEditMessages: messages.canEdit,
          },
    [app, keys, messages, locale],
  );
  // The editors are siblings of the children, not their ancestors: the children keep the plugin's own editor.
  return (
    <>
      <EditorProvider appId={appId} pluginId="" documents={keyDocs} hideBanner>
        <Tap onSlot={setKeys} />
      </EditorProvider>
      {app !== undefined && (
        <EditorProvider key={locale} appId={appId} pluginId="" documents={messageDocs} lockScope={localeScope(locale)}>
          <Tap onSlot={setMessages} />
        </EditorProvider>
      )}
      <AppContext.Provider value={keys}>
        <Context.Provider value={value}>{props.children}</Context.Provider>
      </AppContext.Provider>
    </>
  );
}
