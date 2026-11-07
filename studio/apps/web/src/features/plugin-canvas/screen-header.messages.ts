// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the bar above each screen on the plugin canvas (MR-2). */
export function screenHeaderMessages(i18n: I18n) {
  return {
    bar: (title: string) =>
      i18n._({
        id: "screenHeader.bar",
        message: "Screen {title}",
        values: { title },
        comment: "Accessible name of the bar above a screen on the plugin canvas: its title and its actions.",
      }),
    editTitle: (title: string) =>
      i18n._({
        id: "screenHeader.editTitle",
        message: "Rename {title}",
        values: { title },
        comment: "Accessible name of a screen's title above its frame: a click renames the screen in place.",
      }),
    titleField: i18n._({
      id: "screenHeader.titleField",
      message: "Screen title",
      comment: "Accessible name of the field that renames a screen in place, above its frame.",
    }),
    renameHint: i18n._({
      id: "screenHeader.renameHint",
      message: "Click to rename",
      comment: "Tooltip of a screen's title above its frame.",
    }),
    entry: i18n._({
      id: "screenHeader.entry",
      message: "Entry",
      comment: "Badge after the title of the screen the plugin opens on.",
    }),
    entryHint: i18n._({
      id: "screenHeader.entryHint",
      message: "The plugin opens on this screen",
      comment: "Tooltip of the Entry badge above a screen.",
    }),
    route: (route: string) =>
      i18n._({
        id: "screenHeader.route",
        message: "Route name: {route}",
        values: { route },
        comment: "Tooltip of the route name shown above a screen: the name other screens navigate to it by.",
      }),
    settings: i18n._({
      id: "screenHeader.settings",
      message: "Page settings",
      comment: "Button above a screen: opens the inspector's Page pane for it.",
    }),
    matrix: i18n._({
      id: "screenHeader.matrix",
      message: "Show on every device",
      comment: "Button above a screen: opens the device matrix with this screen in every device frame.",
    }),
    zoom: i18n._({
      id: "screenHeader.zoom",
      message: "Zoom to screen",
      comment: "Button above a screen: zooms the canvas to fit the screen.",
    }),
    more: (title: string) =>
      i18n._({
        id: "screenHeader.more",
        message: "More actions for {title}",
        values: { title },
        comment: "Button above a screen that opens the menu of all its actions.",
      }),
    rename: i18n._({
      id: "screenHeader.rename",
      message: "Rename",
      comment: "Menu item above a screen: rename it in place.",
    }),
    duplicate: i18n._({
      id: "screenHeader.duplicate",
      message: "Duplicate",
      comment: "Menu item above a screen: make a copy of it.",
    }),
    makeEntry: i18n._({
      id: "screenHeader.makeEntry",
      message: "Make it the entry screen",
      comment: "Menu item above a screen: the plugin opens on this screen from now on.",
    }),
    move: i18n._({
      id: "screenHeader.move",
      message: "Move to another plugin…",
      comment: "Menu item above a screen: move it to another plugin of the app.",
    }),
    delete: i18n._({
      id: "screenHeader.delete",
      message: "Delete…",
      comment: "Menu item above a screen: delete it (asks first).",
    }),
  };
}
