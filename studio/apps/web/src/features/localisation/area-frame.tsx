// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Link } from "@tanstack/react-router";
import type { ReactElement, ReactNode } from "react";
import { PageHeader } from "../../components/page.tsx";
import { localisationAreaMessages } from "./area.messages.ts";
import { AreaScope, useArea } from "./area-scope.tsx";

function Body(props: {
  readonly id: string;
  readonly title: string;
  readonly caption: string;
  readonly children: ReactNode;
}): ReactElement {
  const m = localisationAreaMessages(useLingui().i18n);
  const area = useArea();
  const search = { plugin: area.plugin };
  const params = { org: area.org, app: area.app };
  return (
    <section className="page-frame" aria-labelledby={props.id}>
      <PageHeader
        id={props.id}
        title={props.title}
        caption={props.caption}
        nav={
          <nav aria-label={m.navLabel} className="sub-nav">
            <Link to="/o/$org/apps/$app/localisation" params={params} search={search}>
              {m.gridLink}
            </Link>
            <Link to="/o/$org/apps/$app/localisation/heatmap" params={params} search={search}>
              {m.heatmapLink}
            </Link>
          </nav>
        }
      />
      <div className="page-body">{props.children}</div>
    </section>
  );
}

/** A page of the localisation area: the sessions around the heading, the links between the pages and the content. */
export function AreaFrame(props: {
  readonly id: string;
  readonly title: string;
  readonly caption: string;
  readonly children: ReactNode;
}): ReactElement {
  return (
    <AreaScope
      shell={(content) => (
        <section className="page-frame" aria-labelledby={props.id}>
          <PageHeader id={props.id} title={props.title} caption={props.caption} />
          <div className="page-body">{content}</div>
        </section>
      )}
    >
      <Body {...props} />
    </AreaScope>
  );
}
