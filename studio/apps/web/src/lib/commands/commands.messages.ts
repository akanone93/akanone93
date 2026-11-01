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
    readOnly: i18n._({
      id: "cmd.edit.readOnly",
      message: "Read-only: you cannot change this plugin now",
      comment: "Why undo and redo are disabled: the editor is read-only or lost its lock.",
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
    groupScreens: i18n._({ id: "cmd.group.screens", message: "Screens", comment: "Command group heading." }),
    groupAppearance: i18n._({ id: "cmd.group.appearance", message: "Appearance", comment: "Command group heading." }),
    paletteTitle: i18n._({
      id: "cmd.palette.title",
      message: "Search Studio",
      comment: "Title of the search and command palette dialog.",
    }),
    paletteSearch: i18n._({
      id: "cmd.palette.search",
      message: "Search apps, plugins and screens, or type a command",
      comment: "Label and placeholder of the search box of the palette.",
    }),
    paletteSearching: i18n._({
      id: "cmd.palette.searching",
      message: "Searching the organisation",
      comment: "Shown below the local palette results while the server search runs.",
    }),
    paletteSearchFailed: i18n._({
      id: "cmd.palette.search-failed",
      message: "Organisation search is unavailable.",
      comment: "Shown when the server search request fails.",
    }),
    paletteEmpty: i18n._({
      id: "cmd.palette.empty",
      message: "Nothing matches.",
      comment: "Shown when neither a command nor the organisation search matches the text.",
    }),
    paletteOpen: i18n._({
      id: "cmd.palette.trigger",
      message: "Search",
      comment: "Label of the top bar button that opens the search and command palette.",
    }),
    paletteRecent: i18n._({
      id: "cmd.palette.group.recent",
      message: "Recent",
      comment: "Heading of the commands the person ran last, in the palette.",
    }),
    paletteCommands: i18n._({
      id: "cmd.palette.group.commands",
      message: "Commands",
      comment: "Heading of the matching commands in the palette.",
    }),
    scopeAll: i18n._({
      id: "cmd.palette.scope.all",
      message: "Everything",
      comment: "Palette filter: search commands and the organisation.",
    }),
    scopeCommands: i18n._({
      id: "cmd.palette.scope.commands",
      message: "Commands",
      comment: "Palette filter: commands only (also typed as > before the text).",
    }),
    scopeScreens: i18n._({
      id: "cmd.palette.scope.screens",
      message: "Screens",
      comment: "Palette filter: the organisation's screens only (also typed as # before the text).",
    }),
    scopeLabel: i18n._({
      id: "cmd.palette.scope.label",
      message: "Search in",
      comment: "Accessible name of the palette's filter between everything, commands and screens.",
    }),
    hintMove: i18n._({ id: "cmd.palette.hint.move", message: "Move", comment: "Palette footer: the arrow keys move." }),
    hintOpen: i18n._({ id: "cmd.palette.hint.open", message: "Open", comment: "Palette footer: Enter opens." }),
    keyEscape: i18n._({
      id: "cmd.palette.key.escape",
      message: "Esc",
      comment: "The Escape key as written on a key cap, in the palette's footer.",
    }),
    hintClose: i18n._({ id: "cmd.palette.hint.close", message: "Close", comment: "Palette footer: Escape closes." }),
    kind: (kind: string) =>
      ({
        app: i18n._({ id: "cmd.palette.kind.app", message: "Apps", comment: "Group of search results: apps." }),
        plugin: i18n._({ id: "cmd.palette.kind.plugin", message: "Plugins", comment: "Group of search results." }),
        page: i18n._({ id: "cmd.palette.kind.page", message: "Screens", comment: "Group of search results." }),
        component: i18n._({
          id: "cmd.palette.kind.component",
          message: "Components",
          comment: "Group of search results.",
        }),
        template: i18n._({
          id: "cmd.palette.kind.template",
          message: "Templates",
          comment: "Group of search results.",
        }),
        translation_key: i18n._({
          id: "cmd.palette.kind.translation",
          message: "Translations",
          comment: "Group of search results: translation keys.",
        }),
        function: i18n._({
          id: "cmd.palette.kind.function",
          message: "Functions",
          comment: "Group of search results.",
        }),
      })[kind] ?? kind,
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
