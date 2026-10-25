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
import { CommandsProvider, commandMessages, createRegistry, shellCommands } from "../../lib/commands/index.ts";
import { loadUser } from "../../lib/guards.ts";
import { organizationIdOf } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { shellMessages } from "../../shell.messages.ts";
import { useAppliedPreferences } from "../profile/use-applied-preferences.ts";
import { CommandPalette } from "./command-palette.tsx";
import { chromeMessages } from "./shell-chrome.messages.ts";

/** U9.2: the top bar (identity, organisation switcher, profile menu) over the page content. */
/** One empty list, so memoised commands do not rebuild while queries load. */
const NONE: readonly never[] = [];

export function ShellLayout() {
  const { i18n } = useLingui();
  const m = chromeMessages(i18n);
  const { session } = useApp();
  const router = useRouter();
  const org = (useParams({ strict: false }) as { org?: string }).org;
  const prefs = useQuery(PreferencesService.method.getPreferences, {});
  useAppliedPreferences(prefs.data?.preferences);
  const orgs = useTanstackQuery({
    queryKey: ["organisations"],
    queryFn: () => createClient(OrgService, session.transport).listOrganizations({}),
  });
  const organisations = orgs.data?.organizations ?? NONE;
  const currentId = org === undefined ? undefined : organizationIdOf(org);

  const [paletteOpen, setPaletteOpen] = useState(false);
  const user = useTanstackQuery({ queryKey: ["current-user"], queryFn: () => loadUser(session) });
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
  const c = commandMessages(i18n);

  return (
    <CommandsProvider value={value}>
      <div className="flex min-h-screen flex-col">
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
            <button type="button" className="text-body" onClick={() => setPaletteOpen(true)}>
              {c.paletteOpen}
            </button>
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
          <Outlet />
        </main>
        <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />
      </div>
    </CommandsProvider>
  );
}
