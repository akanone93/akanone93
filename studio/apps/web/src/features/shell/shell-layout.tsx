// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useQuery } from "@connectrpc/connect-query";
import { useLingui } from "@lingui/react";
import { OrgService, PreferencesService, SearchService } from "@plux/api";
import { directionOf } from "@plux/i18n";
import { SidebarInset, SidebarProvider } from "@plux/ui/components/sidebar";
import { useQuery as useTanstackQuery } from "@tanstack/react-query";
import { Outlet, useParams, useRouter } from "@tanstack/react-router";
import { type CSSProperties, useMemo, useState } from "react";
import { openSignInPopup, SignedOutDialog } from "../../components/dialogs/index.ts";
import { CommandsProvider, createRegistry, shellCommands } from "../../lib/commands/index.ts";
import { loadedStudioVersion } from "../../lib/server-version.ts";
import { organizationIdOf } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { useCurrentUser } from "../../lib/use-current-user.ts";
import { createPendingCounter, PendingChangesProvider, usePendingTotal } from "../editor/pending-changes.tsx";
import { GetStarted, useOnboarding } from "../onboarding/index.ts";
import { useAppliedPreferences } from "../profile/use-applied-preferences.ts";
import { StudioSidebar } from "./app-sidebar.tsx";
import { CommandPalette } from "./command-palette.tsx";
import { CreateOrganisationDialog } from "./create-organisation-dialog.tsx";
import { hitPath, type OrgSearchFn } from "./palette-search.ts";
import { SiteFooter } from "./site-footer.tsx";
import { SiteHeader } from "./site-header.tsx";
import { StatusChips, UpdateBanner, useOnline } from "./status-chips.tsx";
import { useServerVersion } from "./use-server-version.ts";
import { useStudioEvents } from "./use-studio-events.ts";

/** One empty list, so memoised commands do not rebuild while queries load. */
const NONE: readonly never[] = [];

/** The sidebar's width and the header's height, as shadcn's dashboard-01 sets them. */
const layoutSize = {
  "--sidebar-width": "calc(var(--spacing) * 64)",
  "--header-height": "calc(var(--spacing) * 12)",
} as CSSProperties;

/**
 * U9.2: shadcn's sidebar layout (dashboard-01). The sidebar holds the organisation switcher, the navigation and the
 * account menu; the header above the page holds where the person is, status, search, theme, notifications and help.
 */
export function ShellLayout() {
  const { i18n } = useLingui();
  const { session } = useApp();
  const router = useRouter();
  const { org, app } = useParams({ strict: false }) as { org?: string; app?: string };
  const online = useOnline();
  const events = useStudioEvents();
  const version = useServerVersion(events.state);
  const prefs = useQuery(PreferencesService.method.getPreferences, {});
  useAppliedPreferences(prefs.data?.preferences);
  const orgs = useTanstackQuery({
    queryKey: ["organisations"],
    queryFn: () => createClient(OrgService, session.transport).listOrganizations({}),
  });
  const organisations = orgs.data?.organizations ?? NONE;
  // Outside an organisation (the profile, the installation) the sidebar keeps the home organisation's navigation.
  const currentId = org === undefined ? session.home() : organizationIdOf(org);

  const [paletteOpen, setPaletteOpen] = useState(false);
  const [gettingStartedOpen, setGettingStartedOpen] = useState(false);
  const onboarding = useOnboarding(org !== undefined);
  const [creatingOrganisation, setCreatingOrganisation] = useState(false);
  const [pending] = useState(createPendingCounter);
  const waiting = usePendingTotal(pending);
  const orgSearch = useMemo<OrgSearchFn>(
    () => async (query, kinds, limit) => {
      const res = await createClient(SearchService, session.transport).search({
        query,
        kinds: [...kinds],
        page: { pageSize: limit, pageToken: "" },
      });
      return res.hits.map((h) => ({ kind: h.kind, id: h.id, name: h.name, appId: h.appId, path: h.path }));
    },
    [session],
  );
  const user = useCurrentUser(currentId);
  const permissions = user.data?.permissions ?? NONE;
  const commands = useMemo(
    () =>
      createRegistry(
        shellCommands({
          i18n,
          session,
          navigate: (path) => router.history.push(path),
          organisations: organisations.map((o) => ({ id: o.id, name: o.name })),
          openPalette: () => setPaletteOpen(true),
        }),
      ),
    // The locale is a dependency because Lingui keeps one i18n object across locale changes.
    [i18n, i18n.locale, session, router, organisations],
  );
  const value = useMemo(
    () => ({ registry: commands, states: commands.stateFor(permissions) }),
    [commands, permissions],
  );
  const current = organisations.find((o) => o.id === currentId);
  const signOut = () => void value.states.find((x) => x.command.id === "account.signout")?.command.run();
  const side = directionOf(i18n.locale) === "rtl" ? "right" : "left";

  return (
    <PendingChangesProvider counter={pending}>
      <CommandsProvider value={value}>
        <SidebarProvider style={layoutSize}>
          <StudioSidebar
            variant="inset"
            side={side}
            organisations={organisations}
            currentId={currentId}
            org={org ?? currentId}
            app={app}
            permissions={permissions}
            user={
              user.data?.user === undefined
                ? undefined
                : { name: user.data.user.displayName, email: user.data.user.email }
            }
            onSwitch={(id) => router.history.push(`/o/${id}`)}
            onCreateOrganisation={
              user.data?.installationAdmin === true ? () => setCreatingOrganisation(true) : undefined
            }
            onSignOut={signOut}
            installationAdmin={user.data?.installationAdmin === true}
          />
          <SidebarInset>
            <UpdateBanner loadedVersion={loadedStudioVersion} serverVersion={version?.studio} />
            <SignedOutDialog open={events.state === "ended"} onSignIn={() => openSignInPopup(events.restart)} />
            <SiteHeader
              title={current?.name ?? ""}
              status={<StatusChips online={online} stream={events.state} waiting={waiting} />}
              docsUrl={version?.docs}
              onSearch={() => setPaletteOpen(true)}
              gettingStarted={
                org !== undefined && (
                  <GetStarted
                    org={org}
                    canCreate={user.data?.permissions.includes("app.manage") === true}
                    open={gettingStartedOpen}
                    onOpenChange={setGettingStartedOpen}
                  />
                )
              }
              onShowGettingStarted={
                org !== undefined && onboarding.ready && !onboarding.complete && onboarding.dismissed
                  ? async () => {
                      if (await onboarding.setDismissed(false)) {
                        setGettingStartedOpen(true);
                      }
                    }
                  : undefined
              }
            />
            {/* Every page sits in the same gutter, as shadcn's dashboard-01 lays out its content. */}
            <div className="@container/main flex flex-1 flex-col gap-6 px-4 pt-6 pb-12 lg:px-6">
              <Outlet />
            </div>
            <SiteFooter version={loadedStudioVersion} docsUrl={version?.docs} />
          </SidebarInset>
          {user.data?.installationAdmin === true && (
            <CreateOrganisationDialog open={creatingOrganisation} onOpenChange={setCreatingOrganisation} />
          )}
          <CommandPalette
            open={paletteOpen}
            onOpenChange={setPaletteOpen}
            search={orgSearch}
            onOpenHit={(hit) => {
              const path = org === undefined ? undefined : hitPath(org, hit);
              if (path !== undefined) {
                router.history.push(path);
              }
            }}
          />
        </SidebarProvider>
      </CommandsProvider>
    </PendingChangesProvider>
  );
}
