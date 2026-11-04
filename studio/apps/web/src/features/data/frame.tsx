// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, Select } from "@plux/ui";
import { buttonVariants } from "@plux/ui/components/button";
import { Link, useNavigate } from "@tanstack/react-router";
import type { ReactElement, ReactNode } from "react";
import { PageHeader } from "../../components/page.tsx";
import { dataMessages } from "./data.messages.ts";
import { DataScope, useDataContext, usePluginIds } from "./scope.tsx";

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
  const navigate = useNavigate();
  const ids = usePluginIds(ctx.app).data ?? [ctx.plugin];
  return (
    <section className="page-frame" aria-labelledby={props.id}>
      <PageHeader
        id={props.id}
        title={props.title}
        caption={props.caption}
        actions={
          <div className="w-56">
            <Select
              label={m.pickPlugin}
              options={ids.map((id) => ({ value: id, label: id }))}
              value={ctx.plugin}
              onValueChange={(plugin) =>
                void navigate({ to: ".", resetScroll: false, search: (prev: object) => ({ ...prev, plugin }) })
              }
            />
          </div>
        }
        nav={
          <nav aria-label={m.areaNav} className="sub-nav">
            <Link to="/o/$org/apps/$app/data" params={{ org: ctx.org, app: ctx.app }} search={search}>
              {m.sourcesLink}
            </Link>
            <Link to="/o/$org/apps/$app/data/collections" params={{ org: ctx.org, app: ctx.app }} search={search}>
              {m.collectionsLink}
            </Link>
          </nav>
        }
      />
      <div className="page-body">
        {props.back !== undefined && (
          <Link
            to={props.back.to}
            params={{ org: ctx.org, app: ctx.app }}
            search={search}
            className={buttonVariants({ variant: "ghost", size: "sm", className: "self-start" })}
          >
            <Icon name="arrow-left" data-icon="inline-start" />
            {props.back.label}
          </Link>
        )}
        {props.children}
      </div>
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
    <DataScope
      shell={(content) => (
        <section className="page-frame" aria-labelledby={props.id}>
          <PageHeader id={props.id} title={props.title} caption={props.caption} />
          <div className="page-body">{content}</div>
        </section>
      )}
    >
      <Body {...props} />
    </DataScope>
  );
}
