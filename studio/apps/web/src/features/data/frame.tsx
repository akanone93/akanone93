// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Link } from "@tanstack/react-router";
import type { ReactElement, ReactNode } from "react";
import { dataMessages } from "./data.messages.ts";
import { DataScope, useDataContext } from "./scope.tsx";

/** The heading, caption and a link back, around the content of a page of the area. */
function Body(props: {
  readonly id: string;
  readonly title: string;
  readonly caption: string;
  readonly back?: {
    readonly to: "/o/$org/apps/$app/data" | "/o/$org/apps/$app/data/collections";
    readonly label: string;
  };
  readonly children: ReactNode;
}): ReactElement {
  const m = dataMessages(useLingui().i18n);
  const ctx = useDataContext();
  const search = { env: ctx.env, plugin: ctx.plugin };
  return (
    <section className="flex flex-col gap-4" aria-labelledby={props.id}>
      <h1 id={props.id} className="text-screen">
        {props.title}
      </h1>
      <p className="text-body text-secondary">{props.caption}</p>
      <nav className="flex gap-4">
        <Link className="text-body" to="/o/$org/apps/$app/data" params={{ org: ctx.org, app: ctx.app }} search={search}>
          {m.sourcesLink}
        </Link>
        <Link
          className="text-body"
          to="/o/$org/apps/$app/data/collections"
          params={{ org: ctx.org, app: ctx.app }}
          search={search}
        >
          {m.collectionsLink}
        </Link>
      </nav>
      {props.back !== undefined && (
        <Link className="text-body" to={props.back.to} params={{ org: ctx.org, app: ctx.app }} search={search}>
          {props.back.label}
        </Link>
      )}
      {props.children}
    </section>
  );
}

/** A page of the Data area: the scope (plugin, documents, edit session) around the heading and the content. */
export function Frame(props: {
  readonly id: string;
  readonly title: string;
  readonly caption: string;
  readonly back?: {
    readonly to: "/o/$org/apps/$app/data" | "/o/$org/apps/$app/data/collections";
    readonly label: string;
  };
  readonly children: ReactNode;
}): ReactElement {
  return (
    <DataScope>
      <Body {...props} />
    </DataScope>
  );
}
