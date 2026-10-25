// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the app canvas (C1, STU-030, STU-034, STU-035, STU-071). */
export function appCanvasMessages(i18n: I18n) {
  return {
    title: i18n._({
      id: "appCanvas.title",
      message: "App canvas",
      comment: "Accessible name of the canvas that shows the plugins of an app.",
    }),
    toolbar: i18n._({
      id: "appCanvas.toolbar",
      message: "Canvas actions",
      comment: "Accessible name of the toolbar above the app canvas.",
    }),
    newPlugin: i18n._({
      id: "appCanvas.newPlugin",
      message: "New plugin",
      comment: "Button and dialog title: add a plugin to the app.",
    }),
    newFrame: i18n._({
      id: "appCanvas.newFrame",
      message: "Frame selection",
      comment: "Button: group the selected plugins in a labelled frame.",
    }),
    frameLabel: i18n._({
      id: "appCanvas.frameLabel",
      message: "Frame label",
      comment: "Field label for the name of a frame.",
    }),
    autoLayout: i18n._({
      id: "appCanvas.autoLayout",
      message: "Arrange",
      comment: "Button: lay the plugin boxes out automatically.",
    }),
    zoomToFit: i18n._({
      id: "appCanvas.zoomToFit",
      message: "Zoom to fit",
      comment: "Button: show every plugin box in the view.",
    }),
    settings: i18n._({
      id: "appCanvas.settings",
      message: "Plugin settings",
      comment: "Button and dialog title: change a plugin's name, description, owner and tags.",
    }),
    duplicate: i18n._({
      id: "appCanvas.duplicate",
      message: "Duplicate plugin",
      comment: "Button and dialog title: copy a plugin under a new key.",
    }),
    archive: i18n._({
      id: "appCanvas.archive",
      message: "Archive plugin",
      comment: "Button: archive the selected plugin without deleting it.",
    }),
    restore: i18n._({
      id: "appCanvas.restore",
      message: "Restore plugin",
      comment: "Button: bring an archived plugin back.",
    }),
    setEntry: i18n._({
      id: "appCanvas.setEntry",
      message: "Make entry plugin",
      comment: "Button and dialog title: the plugin whose entry page the app opens.",
    }),
    entryBadge: i18n._({
      id: "appCanvas.entryBadge",
      message: "Entry",
      comment: "Badge on the plugin box of the entry plugin.",
    }),
    archivedBadge: i18n._({
      id: "appCanvas.archivedBadge",
      message: "Archived",
      comment: "Badge on the box of an archived plugin.",
    }),
    name: i18n._({ id: "appCanvas.name", message: "Name", comment: "Field label for a plugin's name." }),
    key: i18n._({
      id: "appCanvas.key",
      message: "Key",
      comment: "Field label for a plugin's key, its stable identifier.",
    }),
    description: i18n._({
      id: "appCanvas.description",
      message: "Description",
      comment: "Field label for a plugin's description.",
    }),
    tags: i18n._({
      id: "appCanvas.tags",
      message: "Tags (comma separated)",
      comment: "Field label for a plugin's tags.",
    }),
    owner: i18n._({
      id: "appCanvas.owner",
      message: "Owning team",
      comment: "Field label for the team that owns a plugin.",
    }),
    save: i18n._({ id: "appCanvas.save", message: "Save", comment: "Dialog button that applies the change." }),
    cancel: i18n._({
      id: "appCanvas.cancel",
      message: "Cancel",
      comment: "Dialog button that closes it without a change.",
    }),
    close: i18n._({ id: "appCanvas.close", message: "Close", comment: "Accessible name of the dialog close button." }),
    empty: i18n._({
      id: "appCanvas.empty",
      message: "This app has no plugins yet.",
      comment: "Shown on the app canvas when the app has no plugins.",
    }),
    failed: i18n._({
      id: "appCanvas.failed",
      message: "The change could not be saved.",
      comment: "Shown when a plugin or canvas change was refused.",
    }),
    readOnly: i18n._({
      id: "appCanvas.readOnly",
      message: "Positions are read-only while someone else edits the app.",
      comment: "Shown when the app-level lock is held by another person.",
    }),
    plugin: (name: string, key: string) =>
      i18n._({
        id: "appCanvas.plugin",
        message: "{name}, plugin {key}",
        comment: "Accessible name of a plugin box.",
        values: { name, key },
      }),
    screens: (n: number) =>
      i18n._({
        id: "appCanvas.screens",
        message: "{n, plural, one {# screen} other {# screens}}",
        comment: "Count of a plugin's screens.",
        values: { n },
      }),
    versions: (n: number) =>
      i18n._({
        id: "appCanvas.versions",
        message: "{n, plural, one {# version} other {# versions}}",
        comment: "Count of a plugin's published versions.",
        values: { n },
      }),
    lockedBy: (who: string) =>
      i18n._({
        id: "appCanvas.lockedBy",
        message: "Locked by {who}",
        comment: "Badge: who holds the editing lock of the plugin.",
        values: { who },
      }),
    unpublished: i18n._({
      id: "appCanvas.unpublished",
      message: "Unpublished changes",
      comment: "Badge: the plugin's draft differs from its newest version.",
    }),
    validationErrors: (n: number) =>
      i18n._({
        id: "appCanvas.validationErrors",
        message: "{n, plural, one {# error} other {# errors}}",
        comment: "Badge: validation errors in a plugin.",
        values: { n },
      }),
    valid: i18n._({ id: "appCanvas.valid", message: "Valid", comment: "Badge: a plugin has no validation errors." }),
    killed: i18n._({
      id: "appCanvas.killed",
      message: "Switched off",
      comment: "Badge: a kill switch turns the plugin off for devices.",
    }),
    bundleSize: (kb: number) =>
      i18n._({
        id: "appCanvas.bundleSize",
        message: "{kb} KB bundle",
        comment: "Size of a plugin's newest bundle in kilobytes.",
        values: { kb },
      }),
    linksOut: (n: number) =>
      i18n._({
        id: "appCanvas.linksOut",
        message: "{n, plural, one {# link out} other {# links out}}",
        comment: "Count of plugins this plugin links to.",
        values: { n },
      }),
    linksIn: (n: number) =>
      i18n._({
        id: "appCanvas.linksIn",
        message: "{n, plural, one {# link in} other {# links in}}",
        comment: "Count of plugins that link to this plugin.",
        values: { n },
      }),
    functions: (n: number) =>
      i18n._({
        id: "appCanvas.functions",
        message: "{n, plural, one {# function} other {# functions}}",
        comment: "Count of functions a plugin uses.",
        values: { n },
      }),
    frame: (label: string) =>
      i18n._({
        id: "appCanvas.frame",
        message: "Frame {label}",
        comment: "Accessible name of a frame around plugin boxes.",
        values: { label },
      }),
    selected: (n: number) =>
      i18n._({
        id: "appCanvas.selected",
        message: "{n, plural, one {# plugin selected} other {# plugins selected}}",
        comment: "Announced when the selection changes.",
        values: { n },
      }),
  };
}
