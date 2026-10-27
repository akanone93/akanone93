// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Link } from "@tanstack/react-router";
import type { ReactElement, ReactNode } from "react";
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
    <section className="flex flex-col gap-4" aria-labelledby={props.id}>
      <h1 id={props.id} className="text-screen">
        {props.title}
      </h1>
      <p className="text-body text-secondary">{props.caption}</p>
      <nav aria-label={m.navLabel} className="flex gap-4">
        <Link className="text-body" to="/o/$org/apps/$app/localisation" params={params} search={search}>
          {m.gridLink}
        </Link>
        <Link className="text-body" to="/o/$org/apps/$app/localisation/heatmap" params={params} search={search}>
          {m.heatmapLink}
        </Link>
      </nav>
      {props.children}
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
    <AreaScope>
      <Body {...props} />
    </AreaScope>
  );
}
