// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useQuery } from "@connectrpc/connect-query";
import { useLingui } from "@lingui/react";
import { OrgService, PreferencesService } from "@plux/api";
import { GlassCapsule, Icon, Menu, Select } from "@plux/ui";
import { useQuery as useTanstackQuery } from "@tanstack/react-query";
import { Outlet, useParams, useRouter } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import { openSignInPopup, SignedOutDialog } from "../../components/dialogs/index.ts";
import {
  CommandsProvider,
  commandMessages,
  createRegistry,
  shellCommands,
  useContributions,
} from "../../lib/commands/index.ts";
import { loadUser } from "../../lib/guards.ts";
import { loadedStudioVersion } from "../../lib/server-version.ts";
import { organizationIdOf } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { shellMessages } from "../../shell.messages.ts";
import { createPendingCounter, PendingChangesProvider, usePendingTotal } from "../editor/pending-changes.tsx";
import { useAppliedPreferences } from "../profile/use-applied-preferences.ts";
import { CommandPalette } from "./command-palette.tsx";
import { HelpMenu } from "./help-menu.tsx";
import { NotificationsPopover } from "./notifications-popover.tsx";
import { chromeMessages } from "./shell-chrome.messages.ts";
import { StatusChips, UpdateBanner, useOnline } from "./status-chips.tsx";
import { useServerVersion } from "./use-server-version.ts";
import { useStudioEvents } from "./use-studio-events.ts";

/** One empty list, so memoised commands do not rebuild while queries load. */
const NONE: readonly never[] = [];

/** U9.2: the top bar (identity, organisation switcher, profile menu) over the page content. */

export function ShellLayout() {
  const { i18n } = useLingui();
  const m = chromeMessages(i18n);
  const { session } = useApp();
  const router = useRouter();
  const org = (useParams({ strict: false }) as { org?: string }).org;
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
  const currentId = org === undefined ? undefined : organizationIdOf(org);

  const [paletteOpen, setPaletteOpen] = useState(false);
  const [pending] = useState(createPendingCounter);
  const waiting = usePendingTotal(pending);
  const user = useTanstackQuery({ queryKey: ["current-user"], queryFn: () => loadUser(session) });
  const permissions = user.data?.permissions ?? NONE;
  const { contributed, Provider: Contributions } = useContributions();
  const commands = useMemo(
    () =>
      createRegistry([
        ...shellCommands({
          i18n,
          session,
          navigate: (path) => router.history.push(path),
          organisations: organisations.map((o) => ({ id: o.id, name: o.name })),
          openPalette: () => setPaletteOpen(true),
        }),
        ...contributed,
      ]),
    // The locale is a dependency because Lingui keeps one i18n object across locale changes.
    [i18n, i18n.locale, session, router, organisations, contributed],
  );
  const value = useMemo(
    () => ({ registry: commands, states: commands.stateFor(permissions) }),
    [commands, permissions],
  );
  const c = commandMessages(i18n);

  return (
    <PendingChangesProvider counter={pending}>
      <CommandsProvider value={value}>
        <div className="flex min-h-screen flex-col">
          <UpdateBanner loadedVersion={loadedStudioVersion} serverVersion={version?.studio} />
          <SignedOutDialog open={events.state === "ended"} onSignIn={() => openSignInPopup(events.restart)} />
          <header>
            <GlassCapsule label={m.topBar} className="m-4 flex items-center gap-4 px-4">
              <a href="/" className="text-title">
                {shellMessages(i18n).appTitle}
              </a>
              {organisations.length > 0 && (
                <Select
                  label={m.organisation}
                  value={currentId ?? ""}
                  placeholder={m.organisation}
                  options={organisations.map((o) => ({ value: o.id, label: o.name }))}
                  onValueChange={(id) => router.history.push(`/o/${id}`)}
                />
              )}
              <span className="flex-1" />
              <StatusChips online={online} stream={events.state} waiting={waiting} />
              <button type="button" className="text-body" onClick={() => setPaletteOpen(true)}>
                {c.paletteOpen}
              </button>
              <NotificationsPopover />
              <HelpMenu docsUrl={version?.docs} />
              <Menu
                trigger={
                  <button type="button" aria-label={m.profile} className="inline-flex items-center">
                    <Icon name="user" size="capsule" />
                  </button>
                }
                entries={[
                  { kind: "item", id: "profile", label: m.profile, onSelect: () => router.history.push("/me") },
                  { kind: "separator", id: "sep" },
                  {
                    kind: "item",
                    id: "signout",
                    label: m.signOut,
                    onSelect: () => void value.states.find((x) => x.command.id === "account.signout")?.command.run(),
                  },
                ]}
              />
            </GlassCapsule>
          </header>
          <main className="flex-1 p-6">
            <Contributions>
              <Outlet />
            </Contributions>
          </main>
          <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />
        </div>
      </CommandsProvider>
    </PendingChangesProvider>
  );
}
