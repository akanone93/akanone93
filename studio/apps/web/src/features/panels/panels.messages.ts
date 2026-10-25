// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the left panels (STU-061, S2.6). */
export function panelsMessages(i18n: I18n) {
  return {
    dock: i18n._({
      id: "panels.dock",
      message: "Left panels",
      comment: "Accessible name of the dock at the left of the canvas.",
    }),
    insertTab: i18n._({ id: "panels.insertTab", message: "Insert", comment: "Tab: add widgets to the screen." }),
    layersTab: i18n._({
      id: "panels.layersTab",
      message: "Layers",
      comment: "Tab: the tree of the nodes of the screens.",
    }),
    screensTab: i18n._({
      id: "panels.screensTab",
      message: "Screens",
      comment: "Tab: the list of the plugin's screens.",
    }),
    selectionTab: i18n._({ id: "panels.selectionTab", message: "Selection", comment: "Tab: the selected nodes." }),
    search: i18n._({
      id: "panels.search",
      message: "Search widgets",
      comment: "Label of the search field of the Insert tab.",
    }),
    layer1: i18n._({
      id: "panels.layer1",
      message: "Widgets",
      comment: "Group of the Insert tab: the basic widgets (Layer 1).",
    }),
    layer2: i18n._({
      id: "panels.layer2",
      message: "Components",
      comment: "Group of the Insert tab: ready-made components (Layer 2).",
    }),
    native: i18n._({
      id: "panels.native",
      message: "Native and custom",
      comment: "Group of the Insert tab: widgets of the host app (native slots) and custom widgets.",
    }),
    noMatches: i18n._({
      id: "panels.noMatches",
      message: "No widget matches.",
      comment: "Shown when the Insert search finds nothing.",
    }),
    insert: (type: string) =>
      i18n._({
        id: "panels.insert",
        message: "Insert {type}",
        values: { type },
        comment: "Accessible name of a button that inserts a widget into the selection.",
      }),
    readOnly: i18n._({
      id: "panels.readOnly",
      message: "Read-only: editing is blocked.",
      comment: "Shown in the panels while the document cannot be edited.",
    }),
    cannotHold: i18n._({
      id: "panels.cannotHold",
      message: "The selection cannot take more nodes.",
      comment: "Shown when a widget cannot be inserted into the selected node.",
    }),
    layersLabel: i18n._({
      id: "panels.layersLabel",
      message: "Layers of the plugin",
      comment: "Accessible name of the layer tree.",
    }),
    pluginRoot: i18n._({
      id: "panels.pluginRoot",
      message: "Plugin",
      comment: "Name of the top level of the layer tree.",
    }),
    screensLabel: i18n._({
      id: "panels.screensLabel",
      message: "Screens",
      comment: "Accessible name of the list of screens.",
    }),
    entryScreen: i18n._({
      id: "panels.entryScreen",
      message: "Entry",
      comment: "Marks the screen the plugin opens first.",
    }),
    noSelection: i18n._({
      id: "panels.noSelection",
      message: "Nothing selected.",
      comment: "Selection tab without a selection.",
    }),
    selectedCount: (count: number) =>
      i18n._({
        id: "panels.selectedCount",
        message: "{count, plural, one {# node selected} other {# nodes selected}}",
        values: { count },
        comment: "Summary of a multiple selection.",
      }),
    selectionLabel: i18n._({
      id: "panels.selectionLabel",
      message: "Selected nodes",
      comment: "Accessible name of the list of selected nodes.",
    }),
  };
}
