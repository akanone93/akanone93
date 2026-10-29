// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { OrgService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { useState } from "react";
import { organizationIdOf } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { useCan } from "../admin/use-admin.ts";
import { AppsArea } from "../apps/index.ts";
import { OnboardingChecklist } from "../onboarding/index.ts";
import { CreateAppDialog } from "./create-app-dialog.tsx";
import { homeMessages } from "./home.messages.ts";

/** O1: the organisation home with its apps. The other cards arrive with their slices (STU-015). */
export function HomePage() {
  const m = homeMessages(useLingui().i18n);
  const { session } = useApp();
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const id = organizationIdOf(org);
  const canCreate = useCan("app.manage");
  const [creating, setCreating] = useState(false);
  const orgs = useQuery({
    queryKey: ["organisations"],
    queryFn: () => createClient(OrgService, session.transport).listOrganizations({}),
  });
  const name = orgs.data?.organizations.find((o) => o.id === id)?.name ?? "";
  return (
    <section aria-labelledby="home-title" className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <h1 id="home-title" className="text-screen">
          {m.title(name)}
        </h1>
        <p className="text-body text-secondary">{m.caption}</p>
      </div>
      <OnboardingChecklist org={org} canCreate={canCreate} onCreate={() => setCreating(true)} />
      <AppsArea org={org} organizationId={id} onCreate={canCreate ? () => setCreating(true) : undefined} />
      {canCreate && <CreateAppDialog open={creating} onOpenChange={setCreating} organizationId={id} org={org} />}
    </section>
  );
}
