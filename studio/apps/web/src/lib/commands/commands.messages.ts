// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the command registry, palette and shortcut sheet (STU-013). */
export function commandMessages(i18n: I18n) {
  return {
    openPalette: i18n._({ id: "cmd.palette.open", message: "Open command palette", comment: "Command title." }),
    goHome: i18n._({ id: "cmd.nav.home", message: "Go to home", comment: "Command title." }),
    goProfile: i18n._({ id: "cmd.nav.profile", message: "Go to profile", comment: "Command title." }),
    goShortcuts: i18n._({ id: "cmd.nav.shortcuts", message: "Show keyboard shortcuts", comment: "Command title." }),
    switchOrg: (name: string) =>
      i18n._({
        id: "cmd.org.switch",
        message: "Switch to {name}",
        comment: "Command title; name is an organisation.",
        values: { name },
      }),
    undo: i18n._({ id: "cmd.edit.undo", message: "Undo", comment: "Command title." }),
    redo: i18n._({ id: "cmd.edit.redo", message: "Redo", comment: "Command title." }),
    redoAlt: i18n._({
      id: "cmd.edit.redo.alt",
      message: "Redo (Ctrl+Y)",
      comment: "Command title; the alternative redo chord.",
    }),
    groupEdit: i18n._({ id: "cmd.group.edit", message: "Edit", comment: "Command group heading." }),
    signOut: i18n._({ id: "cmd.account.signout", message: "Sign out", comment: "Command title." }),
    toggleTheme: i18n._({ id: "cmd.theme.toggle", message: "Toggle light and dark theme", comment: "Command title." }),
    groupNavigate: i18n._({ id: "cmd.group.navigate", message: "Navigate", comment: "Command group heading." }),
    groupOrganisation: i18n._({
      id: "cmd.group.organisation",
      message: "Organisation",
      comment: "Command group heading.",
    }),
    groupAccount: i18n._({ id: "cmd.group.account", message: "Account", comment: "Command group heading." }),
    groupAppearance: i18n._({ id: "cmd.group.appearance", message: "Appearance", comment: "Command group heading." }),
    paletteTitle: i18n._({
      id: "cmd.palette.title",
      message: "Command palette",
      comment: "Title of the palette dialog.",
    }),
    paletteSearch: i18n._({
      id: "cmd.palette.search",
      message: "Type a command",
      comment: "Label of the palette search box.",
    }),
    paletteEmpty: i18n._({
      id: "cmd.palette.empty",
      message: "No command matches.",
      comment: "Shown when the filter matches nothing.",
    }),
    paletteOpen: i18n._({
      id: "cmd.palette.trigger",
      message: "Commands",
      comment: "Label of the top bar button that opens the palette.",
    }),
    close: i18n._({
      id: "cmd.palette.close",
      message: "Close",
      comment: "Accessible name of the dialog close button.",
    }),
    missing: (permission: string) =>
      i18n._({
        id: "cmd.disabled.reason",
        message: "Needs the permission {permission}",
        comment: "Why a command is disabled.",
        values: { permission },
      }),
    shortcutsTitle: i18n._({
      id: "cmd.shortcuts.title",
      message: "Keyboard shortcuts",
      comment: "Heading of the shortcuts page.",
    }),
    shortcutsCaption: i18n._({
      id: "cmd.shortcuts.caption",
      message: "Generated from the commands of this window.",
      comment: "Caption of the shortcuts page.",
    }),
    none: i18n._({
      id: "cmd.shortcuts.none",
      message: "No shortcut",
      comment: "Shown for a command without a shortcut.",
    }),
  };
}
