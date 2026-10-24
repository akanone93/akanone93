// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { OrgService } from "@plux/api";
import { useLingui } from "@lingui/react";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { organizationIdOf } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { homeMessages } from "./home.messages.ts";

/** O1: the organisation home. Its cards arrive with their slices (STU-015). */
export function HomePage() {
  const m = homeMessages(useLingui().i18n);
  const { session } = useApp();
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const id = organizationIdOf(org);
  const orgs = useQuery({
    queryKey: ["organisations"],
    queryFn: () => createClient(OrgService, session.transport).listOrganizations({}),
  });
  const name = orgs.data?.organizations.find((o) => o.id === id)?.name ?? "";
  return (
    <section aria-labelledby="home-title" className="flex flex-col gap-2">
      <h1 id="home-title" className="text-screen">
        {m.title(name)}
      </h1>
      <p className="text-body text-secondary">{m.caption}</p>
    </section>
  );
}
