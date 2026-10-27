// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { PluginService } from "@plux/api";
import { limitDefault } from "@plux/schema";
import type { Problem } from "@plux/validator";
import { useQuery } from "@tanstack/react-query";
import { useParams, useRouteContext, useSearch } from "@tanstack/react-router";
import { createContext, type ReactElement, type ReactNode, useContext, useEffect, useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import type { RouterContext } from "../../router.tsx";
import type { DesignerDocument } from "../designers/use-document.ts";
import { tabEditorSession } from "../editor/document-transport.ts";
import type { EditSession } from "../editor/edit-session.ts";
import { EditorProvider, useEditSession } from "../editor/editor-provider.tsx";
import { UndoManager } from "../editor/undo-stack.ts";
import type { LoadPublished } from "../json-editor/use-published.ts";
import { usePublishedLoader } from "../json-editor/use-published.ts";
import { L10nSessionsProvider, useAppSession } from "../localisation/index.ts";
import type { AppSession } from "../localisation/l10n-sessions.tsx";
import { useAppDocuments, usePluginDocuments } from "../plugin-canvas/use-plugin-documents.ts";
import { useValidation } from "../problems/use-validation.ts";
import { browserValidator, type ValidatorClient } from "../problems/validator-client.ts";
import { dataMessages } from "./data.messages.ts";

/** What the pages of the area share: the documents of the chosen plugin and of the app, and the validator. */
export interface DataContext {
  readonly org: string;
  readonly app: string;
  readonly env: string | undefined;
  readonly plugin: string;
  readonly documents: readonly DesignerDocument[] | undefined;
  readonly appDocuments: readonly DesignerDocument[] | undefined;
  readonly appSession: AppSession | null;
  readonly validatePage: ((file: string, data: string) => Promise<readonly Problem[]>) | undefined;
  readonly tool: ValidatorClient["tool"];
  readonly loadPublished: LoadPublished;
}

function tabStorage(): Storage | undefined {
  try {
    return window.sessionStorage;
  } catch {
    return undefined;
  }
}

/** Hands the plugin's edit pipeline to an observer (tests); renders nothing. */
function ObserveEditor({ observe }: { readonly observe: (edit: EditSession) => void }): null {
  const edit = useEditSession();
  useEffect(() => {
    observe(edit);
  }, [observe, edit]);
  return null;
}

const Ctx = createContext<DataContext | null>(null);

export function useDataContext(): DataContext {
  const ctx = useContext(Ctx);
  if (ctx === null) {
    throw new Error("useDataContext outside DataScope");
  }
  return ctx;
}

function usePluginIds(app: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["data-plugins", app],
    queryFn: async () =>
      (
        await createClient(PluginService, session.transport).listPlugins({ appId: app, page: { pageSize: 100 } })
      ).plugins
        .map((p) => p.id)
        .sort(),
  });
}

function Inner(props: { plugin: string; children: ReactNode }): ReactElement {
  const params = useParams({ strict: false }) as { org?: string; app?: string };
  const env = (useSearch({ strict: false }) as { env?: string }).env;
  const app = params.app ?? "";
  const { createValidator = browserValidator, observeEditor } = useRouteContext({ strict: false }) as Pick<
    RouterContext,
    "createValidator" | "observeEditor"
  >;
  const docs = usePluginDocuments(app, props.plugin);
  const appDocs = useAppDocuments(app);
  const validation = useValidation(createValidator, appDocs.data, docs.data);
  const done = validation.state === "done" ? validation : undefined;
  const appSession = useAppSession();
  const loadPublished = usePublishedLoader(props.plugin);
  const value: DataContext = {
    org: params.org ?? "",
    app,
    env,
    plugin: props.plugin,
    documents: docs.data,
    appDocuments: appDocs.data,
    appSession,
    validatePage: done?.validatePage,
    tool: done?.tool,
    loadPublished,
  };
  return (
    <Ctx.Provider value={value}>
      {observeEditor !== undefined && <ObserveEditor observe={observeEditor} />}
      {props.children}
    </Ctx.Provider>
  );
}

/** The plugin, its documents and the editing session the pages of the Data area work in (the chosen `?plugin`, else the first plugin of the app). */
export function DataScope({ children }: { readonly children: ReactNode }): ReactElement {
  const m = dataMessages(useLingui().i18n);
  const app = (useParams({ strict: false }) as { app?: string }).app ?? "";
  const chosen = (useSearch({ strict: false }) as { plugin?: string }).plugin;
  const ids = usePluginIds(app);
  const appDocs = useAppDocuments(app);
  const docs = usePluginDocuments(app, chosen ?? ids.data?.[0] ?? "");
  const [undo] = useState(() => new UndoManager({ depth: limitDefault("studio.undoDepth"), blocked: () => undefined }));
  const [editorSessionId] = useState(() => tabEditorSession(tabStorage()));
  const plugin = chosen ?? ids.data?.[0];
  if (ids.isPending) {
    return (
      <p role="status" className="text-body text-secondary">
        {m.loading}
      </p>
    );
  }
  if (plugin === undefined) {
    return <p className="text-body text-secondary">{m.noPlugin}</p>;
  }
  return (
    <EditorProvider appId={app} pluginId={plugin} documents={docs.data} undo={undo} editorSessionId={editorSessionId}>
      <L10nSessionsProvider appId={app} documents={appDocs.data}>
        <Inner plugin={plugin}>{children}</Inner>
      </L10nSessionsProvider>
    </EditorProvider>
  );
}
