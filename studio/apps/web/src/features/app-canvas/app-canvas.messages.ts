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
    editFrame: (label: string) =>
      i18n._({
        id: "appCanvas.editFrame",
        message: "Rename or resize frame {label}",
        values: { label },
        comment: "Button: opens the dialog that renames or resizes a frame; label is the frame's name.",
      }),
    frameWidth: i18n._({
      id: "appCanvas.frameWidth",
      message: "Width",
      comment: "Field label for the width of a frame.",
    }),
    frameHeight: i18n._({
      id: "appCanvas.frameHeight",
      message: "Height",
      comment: "Field label for the height of a frame.",
    }),
    noOwner: i18n._({
      id: "appCanvas.noOwner",
      message: "No owning team",
      comment: "Option in the owning-team picker: the plugin has no owning team.",
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
    noScreens: i18n._({
      id: "appCanvas.detail.noScreens",
      message: "No screens yet",
      comment: "Detail card of a plugin: the plugin has no screens, so the mini-map is empty.",
    }),
    miniMap: (screens: string) =>
      i18n._({
        id: "appCanvas.detail.miniMap",
        message: "Screens: {screens}",
        comment: "Accessible name of the mini-map of a plugin's screens; the value lists the screen titles.",
        values: { screens },
      }),
    functionsUsed: (names: string) =>
      i18n._({
        id: "appCanvas.detail.functionsUsed",
        message: "Uses {names}",
        comment: "Detail card of a plugin: the names of the Plux Functions it uses.",
        values: { names },
      }),
    publishedBy: (name: string, when: string) =>
      i18n._({
        id: "appCanvas.detail.publishedBy",
        message: "Last published by {name}, {when}",
        comment: "Detail card of a plugin: who published the newest version, and when.",
        values: { name, when },
      }),
    publishedAt: (when: string) =>
      i18n._({
        id: "appCanvas.detail.publishedAt",
        message: "Last published {when}",
        comment: "Detail card of a plugin: when the newest version was published, the publisher being unknown.",
        values: { when },
      }),
    neverPublished: i18n._({
      id: "appCanvas.detail.neverPublished",
      message: "Not published yet",
      comment: "Detail card of a plugin: it has no published version.",
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
    deletePlugin: i18n._({
      id: "appCanvas.deletePlugin",
      message: "Delete plugin",
      comment:
        "Button: delete the selected plugin; a published plugin is deprecated first, an unpublished one goes to the trash.",
    }),
    deleteConfirm: (name: string) =>
      i18n._({
        id: "appCanvas.deleteConfirm",
        message: "Delete {name}? It moves to the trash for 30 days and can be restored with its links.",
        values: { name },
        comment: "Question that asks to confirm deleting a plugin (GOV-031, A44).",
      }),
    deprecateConfirm: (name: string, links: number) =>
      i18n._({
        id: "appCanvas.deprecateConfirm",
        message:
          "{name} is published. It is marked deprecated, and {links, plural, =0 {nothing links to it} one {# link still points to it} other {# links still point to it}}. It leaves the next release only when nothing links to it; earlier releases keep it.",
        values: { name, links },
        comment: "STU-073: explains that deleting a published plugin deprecates it and says what still links to it.",
      }),
    deprecate: i18n._({
      id: "appCanvas.deprecate",
      message: "Deprecate plugin",
      comment: "Confirm button of the dialog that marks a published plugin deprecated (STU-073).",
    }),
    productionNote: (name: string) =>
      i18n._({
        id: "appCanvas.productionNote",
        message: "{name} is part of an app with a production release. Type its key to confirm.",
        values: { name },
        comment: "STU-074: explains why a destructive action on a plugin asks for its key.",
      }),
    redirectTitle: i18n._({
      id: "appCanvas.redirectTitle",
      message: "Redirect incoming links",
      comment: "Title of the dialog that chooses the screen links into a plugin are redirected to (STU-070).",
    }),
    redirectTarget: i18n._({
      id: "appCanvas.redirectTarget",
      message: "New target screen",
      comment: "Label of the choice of the screen links are redirected to.",
    }),
    redirect: i18n._({
      id: "appCanvas.redirect",
      message: "Redirect",
      comment: "Button that confirms redirecting links to the chosen screen.",
    }),
    changeRefused: i18n._({
      id: "appCanvas.changeRefused",
      message: "The change was not applied because a document changed meanwhile. Nothing was written.",
      comment: "Shown when an atomic change across plugins is refused by the server; nothing was written.",
    }),
    trash: i18n._({
      id: "appCanvas.trash",
      message: "Trash",
      comment: "Button and dialog title: deleted plugins and screens that can still be restored (A44).",
    }),
    trashEmpty: i18n._({
      id: "appCanvas.trashEmpty",
      message: "The trash is empty.",
      comment: "Shown in the trash dialog when nothing was deleted.",
    }),
    trashItem: (name: string, kind: string) =>
      i18n._({
        id: "appCanvas.trashItem",
        message: "{name} ({kind})",
        values: { name, kind },
        comment: "A deleted item in the trash list; kind is app, plugin or page.",
      }),
    trashRestore: (name: string) =>
      i18n._({
        id: "appCanvas.trashRestore",
        message: "Restore {name}",
        values: { name },
        comment: "Accessible name of the button that restores one deleted item with its old IDs.",
      }),
    boxes: i18n._({
      id: "appCanvas.boxes",
      message: "Plugins on the canvas",
      comment: "Accessible name of the list of plugin boxes that screen readers and the keyboard use.",
    }),
    native: i18n._({
      id: "appCanvas.native",
      message: "Native screen",
      comment: "Type of a box on the app canvas that stands for a screen of the host app, not a plugin.",
    }),
    moveTo: (name: string) =>
      i18n._({
        id: "appCanvas.moveTo",
        message: "Move {name} to…",
        values: { name },
        comment:
          "Button: opens a dialog to place a box at exact canvas coordinates (keyboard alternative to dragging).",
      }),
    moveSelection: i18n._({
      id: "appCanvas.moveSelection",
      message: "Move to…",
      comment: "Toolbar button: place the selected box at exact canvas coordinates (keyboard alternative to dragging).",
    }),
    moveX: i18n._({
      id: "appCanvas.moveX",
      message: "X position",
      comment: "Field label: horizontal canvas coordinate of a box, in canvas units.",
    }),
    moveY: i18n._({
      id: "appCanvas.moveY",
      message: "Y position",
      comment: "Field label: vertical canvas coordinate of a box, in canvas units.",
    }),
    moved: (name: string, x: number, y: number) =>
      i18n._({
        id: "appCanvas.moved",
        message: "{name} moved to {x}, {y}",
        values: { name, x, y },
        comment: "Announced after a box was moved with the keyboard; x and y are canvas coordinates.",
      }),
    links: i18n._({
      id: "appCanvas.links",
      message: "Links between plugins",
      comment: "Accessible name of the list of arrows between plugin boxes.",
    }),
    bundle: (from: string, to: string, count: number) =>
      i18n._({
        id: "appCanvas.bundle",
        message: "{from} to {to}, {count, plural, one {# link} other {# links}}",
        values: { from, to, count },
        comment: "One arrow between two boxes: source, target and how many navigation links it bundles.",
      }),
    laneMenu: (title: string) =>
      i18n._({
        id: "appCanvas.laneMenu",
        message: "Link {title}",
        values: { title },
        comment: "Title of the menu of one navigation link, e.g. 'Link Home → Offers'.",
      }),
    laneOpenSource: i18n._({
      id: "appCanvas.laneOpenSource",
      message: "Open source screen",
      comment: "Lane menu action: open the plugin canvas at the screen the link starts from.",
    }),
    laneOpenTarget: i18n._({
      id: "appCanvas.laneOpenTarget",
      message: "Open target screen",
      comment: "Lane menu action: open the plugin canvas at the screen the link goes to.",
    }),
    laneChangeTrigger: i18n._({
      id: "appCanvas.laneChangeTrigger",
      message: "Change trigger",
      comment: "Lane menu action: open the widget whose event starts the link, to change the event.",
    }),
    laneRedirect: i18n._({
      id: "appCanvas.laneRedirect",
      message: "Redirect…",
      comment: "Lane menu action: point this one link at another screen.",
    }),
    laneRemove: i18n._({
      id: "appCanvas.laneRemove",
      message: "Remove link",
      comment: "Lane menu action: delete this one navigation step.",
    }),
    toolSelect: i18n._({
      id: "appCanvas.tool.select",
      message: "Select",
      comment: "Canvas tool that selects and moves plugins; a press on the background draws a selection box.",
    }),
    toolHand: i18n._({
      id: "appCanvas.tool.hand",
      message: "Hand",
      comment: "Canvas tool that pans the canvas by dragging.",
    }),
    tools: i18n._({
      id: "appCanvas.tools",
      message: "Tools",
      comment: "Accessible name of the group of canvas tools (Select, Hand).",
    }),
    keyAndOwner: (key: string, team: string) =>
      i18n._({
        id: "appCanvas.keyAndOwner",
        message: "{key} · {team}",
        values: { key, team },
        comment: "The line under a plugin's name on its box: its key, then the team that owns it (STU-030).",
      }),
    zoomIn: i18n._({ id: "appCanvas.zoom.in", message: "Zoom in", comment: "Button that zooms the canvas in." }),
    zoomOut: i18n._({ id: "appCanvas.zoom.out", message: "Zoom out", comment: "Button that zooms the canvas out." }),
    zoomReset: (percent: number) =>
      i18n._({
        id: "appCanvas.zoom.reset",
        message: "Zoom {percent}%, reset to 100%",
        values: { percent },
        comment: "Accessible name of the zoom percentage button; a click resets the zoom to 100 percent.",
      }),
    zoomPercent: (percent: number) =>
      i18n._({
        id: "appCanvas.zoom.percent",
        message: "{percent}%",
        values: { percent },
        comment: "The canvas zoom as a percentage, e.g. '100%'.",
      }),
    zoomControls: i18n._({
      id: "appCanvas.zoom.controls",
      message: "Zoom",
      comment: "Accessible name of the zoom controls of the canvas.",
    }),
    back: (org: string) =>
      i18n._({
        id: "appCanvas.back",
        message: "Back to {org}",
        values: { org },
        comment: "Button that leaves the app canvas for the organisation's home.",
      }),
    pluginCount: (count: number) =>
      i18n._({
        id: "appCanvas.plugin-count",
        message: "{count, plural, one {# plugin} other {# plugins}}",
        values: { count },
        comment: "Under the app's name on the canvas: how many plugins the app has.",
      }),
    layers: i18n._({
      id: "appCanvas.layers",
      message: "Plugins",
      comment: "Title of the panel that lists the app's plugins and frames beside the canvas.",
    }),
    showOnCanvas: (name: string) =>
      i18n._({
        id: "appCanvas.layers.show",
        message: "Show {name} on the canvas",
        values: { name },
        comment: "Accessible name of a row of the plugins panel: selects the plugin and brings it into view.",
      }),
    framesTitle: i18n._({
      id: "appCanvas.layers.frames",
      message: "Frames",
      comment: "Heading of the frames in the plugins panel.",
    }),
    dockTitle: i18n._({
      id: "appCanvas.dockTitle",
      message: "App canvas",
      comment: "Header of the app canvas's left dock, above the app's name.",
    }),
    dockSummary: (plugins: number, frames: number) =>
      i18n._({
        id: "appCanvas.dockSummary",
        message:
          "{plugins, plural, one {# plugin} other {# plugins}} · {frames, plural, one {# frame} other {# frames}}",
        values: { plugins, frames },
        comment: "Under the app's name in the left dock: how many plugins and frames the canvas has.",
      }),
    overviewTab: i18n._({
      id: "appCanvas.overviewTab",
      message: "Overview",
      comment: "Tab of the app canvas's left dock: figures about the whole app.",
    }),
    searchPlugins: i18n._({
      id: "appCanvas.searchPlugins",
      message: "Search plugins",
      comment: "Label of the search field of the Plugins tab of the app canvas.",
    }),
    pluginFilters: i18n._({
      id: "appCanvas.pluginFilters",
      message: "Show plugins",
      comment: "Accessible name of the row of status filters of the Plugins tab.",
    }),
    pluginFilter: (filter: "all" | "draft" | "errors" | "archived") =>
      i18n._({
        id: "appCanvas.pluginFilter",
        message: "{filter, select, all {All} draft {Unpublished} errors {With errors} archived {Archived} other {All}}",
        values: { filter },
        comment: "A status filter of the Plugins tab of the app canvas.",
      }),
    noPluginMatches: i18n._({
      id: "appCanvas.noPluginMatches",
      message: "No plugin matches.",
      comment: "Shown in the Plugins tab when the search or the filter leaves nothing.",
    }),
    noFrames: i18n._({
      id: "appCanvas.noFrames",
      message: "No frames yet. Select plugins and press F to frame them.",
      comment: "Shown in the Frames tab of the app canvas while there is no frame.",
    }),
    overviewFigure: (figure: "plugins" | "screens" | "frames" | "draft" | "errors" | "archived") =>
      i18n._({
        id: "appCanvas.overviewFigure",
        message:
          "{figure, select, plugins {Plugins} screens {Screens} frames {Frames} draft {Unpublished} errors {With errors} archived {Archived} other {Figure}}",
        values: { figure },
        comment: "Name of a figure in the app canvas's overview.",
      }),
    entryPlugin: i18n._({
      id: "appCanvas.entryPlugin",
      message: "Entry plugin",
      comment: "Label of the app's entry plugin in the overview of the app canvas.",
    }),
    noEntry: i18n._({
      id: "appCanvas.noEntry",
      message: "None chosen",
      comment: "Shown in the overview when the app has no entry plugin.",
    }),
    problemTotals: (errors: number) =>
      i18n._({
        id: "appCanvas.problemTotals",
        message: "{errors, plural, =0 {No validation errors} one {# validation error} other {# validation errors}}",
        values: { errors },
        comment: "The validation errors over every plugin of the app, in the canvas's top bar.",
      }),
    hidePanel: i18n._({
      id: "appCanvas.layers.hide",
      message: "Hide panel",
      comment: "Button that collapses the plugins panel of the canvas.",
    }),
    showPanel: i18n._({
      id: "appCanvas.layers.show-panel",
      message: "Show plugins panel",
      comment: "Button that expands the plugins panel of the canvas.",
    }),
    hint: i18n._({
      id: "appCanvas.hint",
      message: "Drag to move · Space + drag to pan · Ctrl + scroll to zoom · Double-click to open",
      comment: "Short help at the foot of the canvas for the pointer gestures.",
    }),
    emptyHint: i18n._({
      id: "appCanvas.empty.hint",
      message: "Plugins are the building blocks of an app. Add the first one with New plugin in the toolbar.",
      comment: "Explains the empty app canvas.",
    }),
    openPlugin: i18n._({
      id: "appCanvas.open-plugin",
      message: "Open plugin",
      comment: "Button in the plugin's detail panel that opens its plugin canvas.",
    }),
    statScreens: i18n._({
      id: "appCanvas.stat.screens",
      message: "Screens",
      comment: "Figure label in the plugin's detail panel.",
    }),
    statVersions: i18n._({
      id: "appCanvas.stat.versions",
      message: "Versions",
      comment: "Figure label in the plugin's detail panel.",
    }),
    linkCount: (n: number) =>
      i18n._({
        id: "appCanvas.linkCount",
        message: "{n, plural, one {# link} other {# links}}",
        comment: "On a plugin's box: how many navigate links go out of and into the plugin.",
        values: { n },
      }),
    releaseDraft: i18n._({
      id: "appCanvas.release.draft",
      message: "Draft",
      comment: "Chip on a plugin's box: the plugin has never been published.",
    }),
    releaseVersion: (version: string) =>
      i18n._({
        id: "appCanvas.release.version",
        message: "v{version}",
        comment: "Chip on a plugin's box: the plugin's newest published version, e.g. v3.",
        values: { version },
      }),
    pluginActions: i18n._({
      id: "appCanvas.plugin-actions",
      message: "Plugin actions",
      comment: "Accessible name of the actions of the selected plugin in its detail panel.",
    }),
  };
}
