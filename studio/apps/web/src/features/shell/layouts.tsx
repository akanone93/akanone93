// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useQuery } from "@connectrpc/connect-query";
import { PreferencesService } from "@plux/api";
import { Outlet } from "@tanstack/react-router";
import { CommandsProvider, createRegistry } from "../../lib/commands/index.ts";
import { useAppliedPreferences } from "../profile/use-applied-preferences.ts";

/** Public pages (sign-in family): no shell, no organisation (R1). */
export function PublicLayout() {
  return (
    <main className="mx-auto flex min-h-screen max-w-sm flex-col justify-center p-6">
      <Outlet />
    </main>
  );
}

const NO_COMMANDS = { registry: createRegistry([]), states: [] };

/**
 * Canvas layout: an empty shell until the first canvas page lands (L-7). It carries a command
 * scope with no shell commands of its own, so pages register theirs (undo and redo, F3).
 */
export function CanvasLayout() {
  // A canvas page can be the first one loaded: theme, density, language and direction apply here as in the shell (STU-012, STU-014).
  const prefs = useQuery(PreferencesService.method.getPreferences, {});
  useAppliedPreferences(prefs.data?.preferences);
  return (
    <CommandsProvider value={NO_COMMANDS}>
      <main className="h-screen w-screen">
        <Outlet />
      </main>
    </CommandsProvider>
  );
}

/** Page layout for app pages: an empty shell until the first app page lands (L-7). */
export function PageLayout() {
  return (
    <div className="flex flex-col gap-6 p-6">
      <Outlet />
    </div>
  );
}
