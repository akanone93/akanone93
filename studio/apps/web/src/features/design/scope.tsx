// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Link, useParams } from "@tanstack/react-router";
import { createContext, type ReactElement, type ReactNode, useContext, useMemo, useRef } from "react";
import { isObject, type Json, useDocumentValue } from "../designers/use-document.ts";
import { EditorProvider } from "../editor/editor-provider.tsx";
import { LockBanner } from "../editor/lock-ui.tsx";
import { L10nSessionsProvider, useAppSession } from "../localisation/index.ts";
import type { AppSession } from "../localisation/l10n-sessions.tsx";
import { useAppDocuments } from "../plugin-canvas/use-plugin-documents.ts";
import { designMessages } from "./design.messages.ts";
import { flattenTokens } from "./design-model.ts";

/** What the pages of the Design system area share: the app's theme document, edited under the app lock through the edit pipeline. */
export interface DesignContext {
  readonly org: string;
  readonly app: string;
  readonly session: AppSession | null;
  readonly path: string | undefined;
  readonly value: Json | undefined;
  readonly rows: ReturnType<typeof flattenTokens>;
  readonly canEdit: boolean;
  /** Edits the tokens of the theme through the edit pipeline; each call is its own undo step. */
  readonly write: (label: string, recipe: (tokens: Json) => void) => void;
}

const Ctx = createContext<DesignContext | null>(null);

export function useDesign(): DesignContext {
  const ctx = useContext(Ctx);
  if (ctx === null) {
    throw new Error("useDesign outside DesignScope");
  }
  return ctx;
}

function Inner({ children }: { readonly children: ReactNode }): ReactElement {
  const params = useParams({ strict: false }) as { org?: string; app?: string };
  const app = params.app ?? "";
  const docs = useAppDocuments(app);
  const session = useAppSession();
  const { path, value } = useDocumentValue(docs.data, "theme.json", session);
  const canEdit = session?.canEdit ?? false;
  const count = useRef(0);
  const rows = useMemo(() => flattenTokens(value?.["tokens"]), [value]);
  const ctx: DesignContext = {
    org: params.org ?? "",
    app,
    session,
    path,
    value,
    rows,
    canEdit,
    write: (label, recipe) => {
      if (path === undefined || session === null || !canEdit) {
        return;
      }
      count.current += 1;
      session.session.edit(
        path,
        (draft) => {
          if (isObject(draft["tokens"])) {
            recipe(draft["tokens"]);
          }
        },
        { group: `design:${label}:${count.current}` },
      );
    },
  };
  return <Ctx.Provider value={ctx}>{children}</Ctx.Provider>;
}

function Body(props: { id: string; title: string; caption: string; children: ReactNode }): ReactElement {
  const m = designMessages(useLingui().i18n);
  const ctx = useDesign();
  return (
    <section className="flex flex-col gap-4" aria-labelledby={props.id}>
      <h1 id={props.id} className="text-screen">
        {props.title}
      </h1>
      <p className="text-body text-secondary">{props.caption}</p>
      <nav aria-label={m.areaNav} className="flex flex-wrap gap-4">
        <Link className="text-body" to="/o/$org/apps/$app/design" params={{ org: ctx.org, app: ctx.app }}>
          {m.tokensLink}
        </Link>
        <Link className="text-body" to="/o/$org/apps/$app/design/themes" params={{ org: ctx.org, app: ctx.app }}>
          {m.themesLink}
        </Link>
        <Link className="text-body" to="/o/$org/apps/$app/design/typography" params={{ org: ctx.org, app: ctx.app }}>
          {m.typographyLink}
        </Link>
        <Link className="text-body" to="/o/$org/apps/$app/design/icons" params={{ org: ctx.org, app: ctx.app }}>
          {m.iconsLink}
        </Link>
        <Link className="text-body" to="/o/$org/apps/$app/design/library" params={{ org: ctx.org, app: ctx.app }}>
          {m.libraryLink}
        </Link>
      </nav>
      {ctx.session !== null && <LockBanner lock={ctx.session.lock} />}
      {!ctx.canEdit && <p className="text-caption text-secondary">{m.readOnly}</p>}
      {ctx.path === undefined && <p className="text-body text-secondary">{m.noTheme}</p>}
      {props.children}
    </section>
  );
}

/** A page of the Design system area: the theme document and its edit session around the heading and the content. */
export function Frame(props: {
  readonly id: string;
  readonly title: string;
  readonly caption: string;
  readonly children: ReactNode;
}): ReactElement {
  const app = (useParams({ strict: false }) as { app?: string }).app ?? "";
  const docs = useAppDocuments(app);
  return (
    <L10nSessionsProvider appId={app} documents={docs.data}>
      <EditorProvider appId={app} pluginId="" documents={[]} hideBanner>
        <Inner>
          <Body {...props} />
        </Inner>
      </EditorProvider>
    </L10nSessionsProvider>
  );
}
