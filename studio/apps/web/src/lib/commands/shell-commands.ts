// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { FieldMaskSchema } from "@bufbuild/protobuf/wkt";
import { createClient } from "@connectrpc/connect";
import type { I18n } from "@lingui/core";
import { IdentityService, PreferencesSchema, PreferencesService } from "@plux/api";
import { forgetUser } from "../guards.ts";
import type { Session } from "../session.ts";
import { commandMessages } from "./commands.messages.ts";
import type { Command } from "./registry.ts";

export interface ShellCommandDeps {
  readonly i18n: I18n;
  readonly session: Session;
  readonly navigate: (path: string) => void;
  readonly organisations: readonly { id: string; name: string }[];
  readonly openPalette: () => void;
}

/** The commands the shell already offers (S0.6). None calls an RPC that a permission guards beyond sign-in. */
export function shellCommands(d: ShellCommandDeps): readonly Command[] {
  const m = commandMessages(d.i18n);
  const signOut = async () => {
    try {
      await createClient(IdentityService, d.session.transport).logout({});
    } finally {
      d.session.setCsrfToken(undefined);
      forgetUser(d.session);
      d.session.queryClient.clear();
      d.navigate("/signed-out");
    }
  };
  const toggleTheme = async () => {
    const root = document.documentElement;
    const next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
    root.setAttribute("data-theme", next);
    await createClient(PreferencesService, d.session.transport).updatePreferences({
      preferences: create(PreferencesSchema, { theme: next }),
      updateMask: create(FieldMaskSchema, { paths: ["theme"] }),
    });
  };
  return [
    {
      id: "palette.open",
      title: m.openPalette,
      group: "navigate",
      shortcut: "mod+k",
      permission: null,
      run: d.openPalette,
    },
    {
      id: "nav.home",
      title: m.goHome,
      group: "navigate",
      shortcut: "alt+h",
      permission: null,
      run: () => d.navigate("/"),
    },
    {
      id: "nav.profile",
      title: m.goProfile,
      group: "navigate",
      shortcut: "alt+p",
      permission: null,
      run: () => d.navigate("/me"),
    },
    {
      id: "nav.shortcuts",
      title: m.goShortcuts,
      group: "navigate",
      shortcut: "shift+?",
      permission: null,
      run: () => d.navigate("/me/shortcuts"),
    },
    ...d.organisations.map(
      (o): Command => ({
        id: `org.switch.${o.id}`,
        title: m.switchOrg(o.name),
        group: "organisation",
        permission: null,
        run: () => d.navigate(`/o/${o.id}`),
      }),
    ),
    {
      id: "theme.toggle",
      title: m.toggleTheme,
      group: "appearance",
      shortcut: "alt+t",
      permission: null,
      run: toggleTheme,
    },
    { id: "account.signout", title: m.signOut, group: "account", permission: null, run: signOut },
  ];
}
