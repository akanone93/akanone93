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

/** Messages of the right dock: Properties, Accessibility and Docs (S2.6, STU-121). */
export function panelMessages(i18n: I18n) {
  return {
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
