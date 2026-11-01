// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { OrgService } from "@plux/api";
import { Icon } from "@plux/ui";
import { Badge } from "@plux/ui/components/badge";
import { Button } from "@plux/ui/components/button";
import { Kbd } from "@plux/ui/components/kbd";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { domAnimation, LazyMotion } from "motion/react";
import { useMemo, useState } from "react";
import { formatShortcut, useCommands } from "../../lib/commands/index.ts";
import { organizationIdOf } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { useCan } from "../admin/use-admin.ts";
import { AppsArea } from "../apps/index.ts";
import { useAppEntries } from "../apps/use-apps.ts";
import { Rise, RisingWords } from "../shell/brand.tsx";
import { AppStack } from "./app-stack.tsx";
import { CreateAppDialog } from "./create-app-dialog.tsx";
import { homeMessages } from "./home.messages.ts";
import { busiest, orgPulse } from "./org-pulse.ts";
import { Pulse } from "./pulse.tsx";
import "./home.css";

/** How many apps the hero stacks: the layers of Plux's mark, plus one. */
const STACKED = 4;

/**
 * O1: the organisation home (STU-015, U16.3). A hero names the organisation and what it runs, beside its busiest apps
 * stacked as layers; the pulse sums the apps' summaries; the apps follow.
 */
export function HomePage() {
  const { i18n } = useLingui();
  const m = homeMessages(i18n);
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
  const entries = useAppEntries(id);
  const pulse = useMemo(() => orgPulse(entries.data ?? []), [entries.data]);
  const stacked = useMemo(() => busiest(entries.data ?? [], STACKED), [entries.data]);
  const palette = useCommands().states.find((s) => s.command.id === "palette.open")?.command;
  const devices = new Intl.NumberFormat(i18n.locale).format(pulse.devices);
  const title = m.title(name);
  const apple = typeof navigator !== "undefined" && /mac|iphone|ipad/i.test(navigator.platform);
  return (
    <LazyMotion features={domAnimation} strict>
      <div className="@container/main flex flex-1 flex-col">
        <section aria-labelledby="home-title" className="flex flex-col gap-6 md:gap-8">
          <div className="grid items-center gap-6 lg:grid-cols-12">
            <div className="flex min-w-0 flex-col gap-5 lg:col-span-7">
              <Rise delay={0}>
                <Badge variant="outline">
                  <span className="home-eyebrow-dot" />
                  {m.eyebrow}
                </Badge>
              </Rise>
              <div className="flex flex-col gap-2">
                <h1 id="home-title" className="text-4xl font-semibold tracking-tight text-balance md:text-5xl">
                  {name === "" ? title : <RisingWords text={title} from={0} />}
                </h1>
                {entries.isSuccess && (
                  <p className="text-2xl font-medium tracking-tight text-muted-foreground md:text-3xl">
                    <RisingWords
                      text={pulse.apps === 0 ? m.heroEmpty : m.heroCount(pulse.apps, devices)}
                      from={title.split(" ").length}
                    />
                  </p>
                )}
              </div>
              <Rise delay={0.6}>
                <p className="max-w-xl text-base text-muted-foreground md:text-lg">{m.lead}</p>
              </Rise>
              <Rise delay={0.8} className="flex flex-wrap items-center gap-3">
                {canCreate && pulse.apps > 0 && (
                  <Button size="lg" onClick={() => setCreating(true)}>
                    <Icon name="plus" data-icon="inline-start" />
                    {m.createApp}
                  </Button>
                )}
                {palette !== undefined && (
                  <Button size="lg" variant="outline" onClick={() => void palette.run()}>
                    <Icon name="magnifying-glass" data-icon="inline-start" />
                    {m.search}
                    {palette.shortcut !== undefined && <Kbd>{formatShortcut(palette.shortcut, apple)}</Kbd>}
                  </Button>
                )}
              </Rise>
            </div>
            <div className="hidden lg:col-span-5 lg:block">
              <AppStack entries={entries.isSuccess ? stacked : []} />
            </div>
          </div>
          {entries.isSuccess && pulse.apps > 0 && (
            <Rise delay={1}>
              <Pulse pulse={pulse} />
            </Rise>
          )}
          <AppsArea org={org} organizationId={id} onCreate={canCreate ? () => setCreating(true) : undefined} />
          {canCreate && <CreateAppDialog open={creating} onOpenChange={setCreating} organizationId={id} org={org} />}
        </section>
      </div>
    </LazyMotion>
  );
}
