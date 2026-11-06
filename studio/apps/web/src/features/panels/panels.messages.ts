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
    kindAll: i18n._({
      id: "panels.kind.all",
      message: "All",
      comment: "Filter of the Insert gallery: every kind of widget.",
    }),
    kindLayout: i18n._({
      id: "panels.kind.layout",
      message: "Layout",
      comment: "Kind of widget in the Insert gallery: rows, columns, stacks, scrolling and structure.",
    }),
    kindContent: i18n._({
      id: "panels.kind.content",
      message: "Content",
      comment: "Kind of widget in the Insert gallery: text, images, icons and other display widgets.",
    }),
    kindInput: i18n._({
      id: "panels.kind.input",
      message: "Inputs",
      comment: "Kind of widget in the Insert gallery: fields, switches, sliders and forms.",
    }),
    kindInteraction: i18n._({
      id: "panels.kind.interaction",
      message: "Interaction",
      comment: "Kind of widget in the Insert gallery: buttons, gestures, dialogs and feedback.",
    }),
    kindNavigation: i18n._({
      id: "panels.kind.navigation",
      message: "Navigation",
      comment: "Kind of widget in the Insert gallery: app bars, tabs, drawers and navigation bars.",
    }),
    kindMotion: i18n._({
      id: "panels.kind.motion",
      message: "Motion",
      comment: "Kind of widget in the Insert gallery: animations and transitions.",
    }),
    kindPlatform: i18n._({
      id: "panels.kind.platform",
      message: "Platform",
      comment: "Kind of widget in the Insert gallery: iOS-style, device and host-app widgets.",
    }),
    kindMore: i18n._({
      id: "panels.kind.more",
      message: "More",
      comment: "Kind of widget in the Insert gallery: everything without another kind.",
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
    editorTitle: i18n._({
      id: "panels.editorTitle",
      message: "Plugin editor",
      comment: "Header of the plugin editor's left dock, above the plugin's name.",
    }),
    editorSummary: (screens: number, widgets: number) =>
      i18n._({
        id: "panels.editorSummary",
        message:
          "{screens, plural, one {# screen} other {# screens}} · {widgets, plural, one {# widget} other {# widgets}}",
        values: { screens, widgets },
        comment: "Under the plugin's name in the left dock: how many screens and widgets the plugin has.",
      }),
    categories: i18n._({
      id: "panels.categories",
      message: "Categories",
      comment: "Foldable section of the Insert tab: one tile per kind of widget, to show only that kind.",
    }),
    showingKind: (kind: string) =>
      i18n._({
        id: "panels.showingKind",
        message: "Only {kind}",
        values: { kind },
        comment: "Chip in the Insert tab while one kind of widget is shown; pressing it shows every kind again.",
      }),
    showAllKinds: i18n._({
      id: "panels.showAllKinds",
      message: "Show every kind",
      comment: "Accessible name of the chip that removes the kind filter of the Insert tab.",
    }),
    recent: i18n._({
      id: "panels.recent",
      message: "Recently used",
      comment: "Section of the Insert tab: the widgets inserted last.",
    }),
    tileHint: i18n._({
      id: "panels.tileHint",
      message: "Click to insert into the selection, or drag onto a screen.",
      comment: "Last line of a widget's details in the Insert tab.",
    }),
    tileFacts: (props: number, events: number) =>
      i18n._({
        id: "panels.tileFacts",
        message:
          "{props, plural, one {# property} other {# properties}} · {events, plural, one {# event} other {# events}}",
        values: { props, events },
        comment: "A widget's details in the Insert tab: how many properties and events it has.",
      }),
    filterLayers: i18n._({
      id: "panels.filterLayers",
      message: "Filter layers",
      comment: "Label of the field that filters the layer tree by name, type or text.",
    }),
    expandAll: i18n._({
      id: "panels.expandAll",
      message: "Expand all",
      comment: "Button of the Layers tab: opens every row of the tree.",
    }),
    collapseAll: i18n._({
      id: "panels.collapseAll",
      message: "Collapse all",
      comment: "Button of the Layers tab: closes every row of the tree.",
    }),
    noLayers: i18n._({
      id: "panels.noLayers",
      message: "No layer matches the filter.",
      comment: "Shown in the Layers tab when the filter matches no row.",
    }),
    clearSelection: i18n._({
      id: "panels.clearSelection",
      message: "Clear selection",
      comment: "Button of the selection card in the Layers tab.",
    }),
    foldDock: i18n._({
      id: "panels.foldDock",
      message: "Fold the panel to its rail",
      comment: "Button in the header of a canvas's left dock: hides the pane and keeps the rail; a tab opens it again.",
    }),
    unfoldDock: i18n._({
      id: "panels.unfoldDock",
      message: "Open the panel",
      comment: "Button beside a canvas's folded left dock: opens the dock again.",
    }),
    layerName: i18n._({
      id: "panels.layerName",
      message: "Layer name",
      comment: "Label of the field that renames a layer in the layer tree.",
    }),
    layerHide: i18n._({
      id: "panels.layerHide",
      message: "Hide on the canvas (Ctrl+Shift+H)",
      comment: "Tooltip of a layer's eye: hides the widget on the canvas only; the device is not affected.",
    }),
    layerShow: i18n._({
      id: "panels.layerShow",
      message: "Show on the canvas (Ctrl+Shift+H)",
      comment: "Tooltip of a hidden layer's eye.",
    }),
    layerLock: i18n._({
      id: "panels.layerLock",
      message: "Lock (Ctrl+Shift+L)",
      comment: "Tooltip of a layer's lock: the widget can no longer be picked on the canvas or edited.",
    }),
    layerUnlock: i18n._({
      id: "panels.layerUnlock",
      message: "Unlock (Ctrl+Shift+L)",
      comment: "Tooltip of a locked layer's lock.",
    }),
    layerHidden: i18n._({
      id: "panels.layerHidden",
      message: "hidden",
      comment: "Read out after a layer's name when it is hidden on the canvas.",
    }),
    layerLocked: i18n._({
      id: "panels.layerLocked",
      message: "locked",
      comment: "Read out after a layer's name when it is locked.",
    }),
    lockedNote: i18n._({
      id: "panels.lockedNote",
      message: "This widget is locked. Unlock it in Layers to edit it.",
      comment: "Shown in the inspector when the selection holds a locked widget.",
    }),
    showSelection: i18n._({
      id: "panels.showSelection",
      message: "Show on canvas",
      comment: "Button of the selection card in the Layers tab: brings the selected widget into view.",
    }),
  };
}

/** Messages of the right dock: Properties, Accessibility and Docs (S2.6, STU-121). */
export function panelMessages(i18n: I18n) {
  return {
    filterProps: i18n._({
      id: "panels.filterProps",
      message: "Filter properties",
      comment: "Placeholder and accessible name of the field that narrows the inspector's properties by name.",
    }),
    setOnly: i18n._({
      id: "panels.setOnly",
      message: "Set only",
      comment: "Toggle that shows only the properties that have a value.",
    }),
    noPropMatch: i18n._({
      id: "panels.noPropMatch",
      message: "No property matches.",
      comment: "Shown when the property filter or Set only leaves nothing to show.",
    }),
    modeOf: (name: string, mode: string) =>
      i18n._({
        id: "panels.modeOf",
        message: "How {name} is given: {mode}",
        comment:
          "Accessible name of the button beside a text property that switches it between a literal value, a translation key and a PXL expression; mode is the current one.",
        values: { name, mode },
      }),
    clearProp: (name: string) =>
      i18n._({
        id: "panels.clearProp",
        message: "Clear {name}",
        comment: "Accessible name of the button that removes a property's value, so the widget uses its default.",
        values: { name },
      }),
    editorsTab: i18n._({
      id: "panels.editorsTab",
      message: "Editors",
      comment: "Tab of the inspector's rail that lists the full-screen editors.",
    }),
    editorsIntro: i18n._({
      id: "panels.editorsIntro",
      message: "Each editor opens over the canvas; Close or Escape returns here.",
      comment: "Line above the list of full-screen editors in the inspector.",
    }),
    editorGroupData: i18n._({
      id: "panels.editorGroup.data",
      message: "Data",
      comment: "Heading of the editors for state, data sources and the database.",
    }),
    editorGroupLogic: i18n._({
      id: "panels.editorGroup.logic",
      message: "Logic",
      comment: "Heading of the editors for action graphs and Rust functions.",
    }),
    editorGroupContent: i18n._({
      id: "panels.editorGroup.content",
      message: "Content and look",
      comment: "Heading of the editors for JSON, translations and the theme.",
    }),
    groupLayout: i18n._({
      id: "panel.group.layout",
      message: "Layout",
      comment: "Section of the inspector's properties: size, spacing, alignment and position.",
    }),
    groupAppearance: i18n._({
      id: "panel.group.appearance",
      message: "Appearance",
      comment: "Section of the inspector's properties: colour, decoration, style and type.",
    }),
    groupContent: i18n._({
      id: "panel.group.content",
      message: "Content",
      comment: "Section of the inspector's properties: text, values, icons, images and children.",
    }),
    groupBehaviour: i18n._({
      id: "panel.group.behaviour",
      message: "Behaviour",
      comment: "Section of the inspector's properties: enabled, input rules, limits and events.",
    }),
    groupOther: i18n._({
      id: "panel.group.other",
      message: "Other",
      comment: "Section of the inspector's properties: everything without another section.",
    }),
    editors: i18n._({
      id: "panel.editors",
      message: "Editors",
      comment:
        "Heading of the inspector's row of buttons that open the full-screen editors (JSON, localisation, state, data, theme, action graph, Rust).",
    }),

    editorShort: (editor: "json" | "l10n" | "state" | "data" | "db" | "theme" | "graph" | "rust") =>
      i18n._({
        id: "panel.editorShort",
        message:
          "{editor, select, json {JSON} l10n {Translations} state {State} data {Data sources} db {Database} theme {Theme} graph {Action graph} rust {Rust} other {Editor}}",
        values: { editor },
        comment:
          "Short name under the icon of a full-screen editor in the inspector; its button's name is the full phrase.",
      }),
    overview: i18n._({
      id: "panel.overview",
      message: "Plugin overview",
      comment: "Heading of the inspector while nothing is selected: figures about the whole plugin.",
    }),
    overviewFigure: (figure: "screens" | "widgets" | "state" | "data" | "errors" | "warnings") =>
      i18n._({
        id: "panel.overviewFigure",
        message:
          "{figure, select, screens {Screens} widgets {Widgets} state {State entries} data {Data sources} errors {Errors} warnings {Warnings} other {Figure}}",
        values: { figure },
        comment: "Name of a figure in the inspector's plugin overview.",
      }),
    overviewScreens: i18n._({
      id: "panel.overviewScreens",
      message: "Screens of the plugin",
      comment: "Heading of the list of the plugin's screens in the inspector's overview.",
    }),
    entry: i18n._({
      id: "panel.entry",
      message: "Entry",
      comment: "Mark beside the plugin's entry screen in the inspector's overview.",
    }),
    multiTitle: (count: number) =>
      i18n._({
        id: "panel.multiTitle",
        message: "{count, plural, one {# widget} other {# widgets}}",
        values: { count },
        comment: "Title of the inspector's header while several widgets are selected.",
      }),
    foldInspector: i18n._({
      id: "panel.foldInspector",
      message: "Fold the inspector",
      comment: "Button in the inspector's header: folds the right dock to a slim strip.",
    }),
    openInspector: i18n._({
      id: "panel.openInspector",
      message: "Open the inspector",
      comment: "Button of the folded inspector: opens the right dock again.",
    }),
    detailsSection: i18n._({
      id: "panel.detailsSection",
      message: "Details",
      comment: "Heading of the inspector's foldable section with the tabs (Properties, Actions, JSON and the others).",
    }),
    dock: i18n._({
      id: "panels.inspector",
      message: "Inspector",
      comment: "Accessible name of the right dock that holds Properties, Accessibility and Docs.",
    }),
    tabProperties: i18n._({
      id: "panels.tabProperties",
      message: "Properties",
      comment: "Label of the right dock tab that edits the selected widget's properties.",
    }),
    tabAccessibility: i18n._({
      id: "panels.tabAccessibility",
      message: "Accessibility",
      comment: "Label of the right dock tab for the selected widget's screen reader semantics.",
    }),
    tabDocs: i18n._({
      id: "panels.tabDocs",
      message: "Docs",
      comment: "Label of the right dock tab that documents the selected widget.",
    }),
    nothing: i18n._({
      id: "panels.nothing",
      message: "Select a widget on the canvas to edit it.",
      comment: "Hint in the right dock when nothing is selected.",
    }),
    mixed: i18n._({
      id: "panels.mixed",
      message: "Mixed",
      comment: "Shown in a property field when the selected widgets have different values.",
    }),
    readOnly: i18n._({
      id: "panels.inspectorReadOnly",
      message: "Read only: you do not hold the edit lock.",
      comment: "Hint above the properties when editing is blocked.",
    }),
    breakpoint: i18n._({
      id: "panels.breakpoint",
      message: "Size class",
      comment: "Label of the control that picks the window size class an edit applies to.",
    }),
    compact: i18n._({
      id: "panels.compact",
      message: "Compact",
      comment: "Window size class under 600 dp; the base values.",
    }),
    medium: i18n._({ id: "panels.medium", message: "Medium", comment: "Window size class from 600 to 839 dp." }),
    expanded: i18n._({ id: "panels.expanded", message: "Expanded", comment: "Window size class from 840 dp." }),
    overridden: i18n._({
      id: "panels.overridden",
      message: "Overridden for this size class",
      comment: "Marks a property whose value differs from the base for the chosen size class.",
    }),
    reset: i18n._({
      id: "panels.reset",
      message: "Reset to base",
      comment: "Button that removes a size class override.",
    }),
    resetOf: (name: string) =>
      i18n._({
        id: "panels.resetOf",
        message: "Reset {name} to base",
        comment: "Accessible name of the button that removes the override of one property.",
        values: { name },
      }),
    unset: i18n._({
      id: "panels.unset",
      message: "Not set",
      comment: "Shown for an optional property that has no value.",
    }),
    clear: (name: string) =>
      i18n._({
        id: "panels.clear",
        message: "Clear {name}",
        comment: "Accessible name of the button that removes a property value.",
        values: { name },
      }),
    useToken: i18n._({
      id: "panels.useToken",
      message: "Theme token",
      comment: "Label of the picker that binds a property to a design token.",
    }),
    useKey: i18n._({
      id: "panels.useKey",
      message: "Translation key",
      comment: "Label of the picker that binds a text to a translation key.",
    }),
    literal: i18n._({
      id: "panels.literal",
      message: "Literal value",
      comment: "Option that sets a fixed value instead of a binding.",
    }),
    pxl: i18n._({ id: "panels.pxl", message: "PXL expression", comment: "Label of the field for a PXL expression." }),
    pxlResultType: (type: string) =>
      i18n._({
        id: "panels.pxlResultType",
        message: "Result type: {type}",
        values: { type },
        comment: "Shown under a valid PXL expression: the type it evaluates to, e.g. int or list<string>.",
      }),
    pxlDrawn: i18n._({
      id: "panels.pxlDrawn",
      message: "The canvas shows",
      comment: "Under a bound expression, before the design-time value the canvas draws for it (a mock or a default).",
    }),
    pxlChecking: i18n._({
      id: "panels.pxlChecking",
      message: "Checking...",
      comment: "Shown while the validator checks an expression.",
    }),
    alignment: i18n._({ id: "panels.alignment", message: "Alignment", comment: "Label of the alignment picker." }),
    left: i18n._({ id: "panels.left", message: "Left", comment: "Edge inset on the left." }),
    top: i18n._({ id: "panels.top", message: "Top", comment: "Edge inset at the top." }),
    right: i18n._({ id: "panels.right", message: "Right", comment: "Edge inset on the right." }),
    bottom: i18n._({ id: "panels.bottom", message: "Bottom", comment: "Edge inset at the bottom." }),
    addItem: i18n._({
      id: "panels.addItem",
      message: "Add item",
      comment: "Button that appends an item to a list property.",
    }),
    removeItem: (n: number) =>
      i18n._({
        id: "panels.removeItem",
        message: "Remove item {n}",
        comment: "Accessible name of the button that removes a list item.",
        values: { n },
      }),
    field: (name: string) =>
      i18n._({
        id: "panels.field",
        message: "Field {name}",
        comment: "Accessible name of a nested field.",
        values: { name },
      }),
    semLabel: i18n._({
      id: "panels.semLabel",
      message: "Semantics label",
      comment: "Label of the field for the text a screen reader announces.",
    }),
    semHint: i18n._({
      id: "panels.semHint",
      message: "Hint",
      comment: "Label of the field for the screen reader hint.",
    }),
    semHeader: i18n._({
      id: "panels.semHeader",
      message: "Heading",
      comment: "Checkbox that marks the widget as a heading.",
    }),
    semButton: i18n._({
      id: "panels.semButton",
      message: "Button",
      comment: "Checkbox that marks the widget as a button.",
    }),
    semLive: i18n._({
      id: "panels.semLive",
      message: "Live region",
      comment: "Checkbox that makes screen readers announce changes.",
    }),
    semGroup: i18n._({
      id: "panels.semGroup",
      message: "Group",
      comment: "Switch: screen readers read the widget and its children as one (A11Y-006).",
    }),
    semDecorative: i18n._({
      id: "panels.semDecorative",
      message: "Decorative",
      comment: "Switch: an image that carries no meaning, which screen readers skip (A11Y-002).",
    }),
    semFocusOrder: i18n._({
      id: "panels.semFocusOrder",
      message: "Focus order",
      comment: "Where the widget comes in the order focus moves through the screen (A11Y-006).",
    }),
    semFocusAuto: i18n._({
      id: "panels.semFocusAuto",
      message: "Reading order",
      comment: "Placeholder: no explicit focus order.",
    }),
    semHeadingLevel: i18n._({
      id: "panels.semHeadingLevel",
      message: "Heading level",
      comment: "A heading's level, 1 to 6.",
    }),
    semExclude: i18n._({
      id: "panels.semExclude",
      message: "Exclude from semantics",
      comment: "Checkbox that hides the widget from screen readers.",
    }),
    role: i18n._({
      id: "panels.role",
      message: "Role",
      comment: "Label of the role the descriptor assigns to the widget.",
    }),
    interactive: i18n._({
      id: "panels.interactive",
      message: "Interactive",
      comment: "Shown when the widget can be operated.",
    }),
    notInteractive: i18n._({
      id: "panels.notInteractive",
      message: "Not interactive",
      comment: "Shown when the widget cannot be operated.",
    }),
    minTarget: i18n._({
      id: "panels.minTarget",
      message: "Needs a touch target of at least 44 by 44 dp.",
      comment: "Shown when the widget must keep a minimum touch target.",
    }),
    a11yProblems: i18n._({
      id: "panels.a11yProblems",
      message: "Accessibility problems",
      comment: "Heading of the list of accessibility findings for the selected widget.",
    }),
    a11yNone: i18n._({
      id: "panels.a11yNone",
      message: "No accessibility problems for this widget.",
      comment: "Shown when the validator found no accessibility problem.",
    }),
    docsProps: i18n._({
      id: "panels.docsProps",
      message: "Properties",
      comment: "Heading of the list of properties in the docs tab.",
    }),
    docsEvents: i18n._({
      id: "panels.docsEvents",
      message: "Events",
      comment: "Heading of the list of events in the docs tab.",
    }),
    docsSlots: i18n._({
      id: "panels.docsSlots",
      message: "Slots",
      comment: "Heading of the list of slots in the docs tab.",
    }),
    docsRequired: i18n._({
      id: "panels.docsRequired",
      message: "required",
      comment: "Marks a required property in the docs tab.",
    }),
    docsDeprecated: i18n._({
      id: "panels.docsDeprecated",
      message: "deprecated",
      comment: "Marks a deprecated property in the docs tab.",
    }),
    multi: (count: number) =>
      i18n._({
        id: "panels.multi",
        message: "{count} widgets selected; showing shared properties.",
        comment: "Shown when several widgets are selected.",
        values: { count },
      }),
  };
}
