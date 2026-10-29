// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the remaining panel tabs (STU-061, STU-062, S5.6). */
export function tabMessages(i18n: I18n) {
  return {
    stateTab: i18n._({
      id: "panelTabs.stateTab",
      message: "State",
      comment: "Tab of the left panel: the plugin's state entries.",
    }),
    dataTab: i18n._({
      id: "panelTabs.dataTab",
      message: "Data",
      comment: "Tab of the left panel: the plugin's data sources and collections.",
    }),
    stateSummary: (count: number) =>
      i18n._({
        id: "panelTabs.stateSummary",
        message: "{count, plural, one {# state entry} other {# state entries}}",
        values: { count },
        comment: "Summary line of the State tab; the count is the number of state entries of the plugin.",
      }),
    stateComputed: (count: number) =>
      i18n._({
        id: "panelTabs.stateComputed",
        message: "{count, plural, one {# computed value} other {# computed values}}",
        values: { count },
        comment: "Summary line of the State tab; the count is the number of state entries that are computed.",
      }),
    stateEmpty: i18n._({
      id: "panelTabs.stateEmpty",
      message: "The plugin has no state yet.",
      comment: "Shown in the State tab when the plugin declares no state.",
    }),
    stateRename: (name: string) =>
      i18n._({
        id: "panelTabs.stateRename",
        message: "Name of state entry {name}",
        values: { name },
        comment: "Accessible name of the field that renames a state entry; the placeholder is its current name.",
      }),
    stateDefault: (name: string) =>
      i18n._({
        id: "panelTabs.stateDefault",
        message: "Initial value of {name}",
        values: { name },
        comment: "Accessible name of the field that edits a state entry's initial value; the placeholder is its name.",
      }),
    stateNameProblem: i18n._({
      id: "panelTabs.stateNameProblem",
      message: "Use a unique name in lowerCamelCase.",
      comment: "Shown beside a state entry name that is empty, malformed or already taken.",
    }),
    stateNotJson: i18n._({
      id: "panelTabs.stateNotJson",
      message: "The initial value is not valid JSON.",
      comment: "Shown beside a state entry's initial value that does not parse as JSON.",
    }),
    stateAddForm: i18n._({
      id: "panelTabs.stateAddForm",
      message: "Add a state entry",
      comment: "Accessible name of the form that adds a state entry.",
    }),
    stateAddName: i18n._({
      id: "panelTabs.stateAddName",
      message: "Name of the new state entry",
      comment: "Accessible name of the field for the name of a new state entry.",
    }),
    stateAdd: i18n._({
      id: "panelTabs.stateAdd",
      message: "Add",
      comment: "Button that adds a state entry.",
    }),
    stateList: i18n._({
      id: "panelTabs.stateList",
      message: "State entries",
      comment: "Accessible name of the list of state entries.",
    }),
    openState: i18n._({
      id: "panelTabs.openState",
      message: "Open the state designer",
      comment: "Button of the State tab that opens the full-screen state designer.",
    }),
    dataSources: (count: number) =>
      i18n._({
        id: "panelTabs.dataSources",
        message: "{count, plural, one {# data source} other {# data sources}}",
        values: { count },
        comment: "Summary line of the Data tab; the count is the number of data sources.",
      }),
    dataMocks: (count: number) =>
      i18n._({
        id: "panelTabs.dataMocks",
        message: "{count, plural, one {# mock} other {# mocks}}",
        values: { count },
        comment: "Summary line of the Data tab; the count is the number of data sources that have a mock response.",
      }),
    dataCollections: (count: number) =>
      i18n._({
        id: "panelTabs.dataCollections",
        message: "{count, plural, one {# collection} other {# collections}}",
        values: { count },
        comment: "Summary line of the Data tab; the count is the number of database collections.",
      }),
    dataEmpty: i18n._({
      id: "panelTabs.dataEmpty",
      message: "The plugin has no data sources or collections yet.",
      comment: "Shown in the Data tab when the plugin declares neither.",
    }),
    dataList: i18n._({
      id: "panelTabs.dataList",
      message: "Data sources",
      comment: "Accessible name of the list of data sources.",
    }),
    openData: i18n._({
      id: "panelTabs.openData",
      message: "Open the data source designer",
      comment: "Button of the Data tab that opens the full-screen data source designer.",
    }),
    tabJson: i18n._({
      id: "panelTabs.tabJson",
      message: "JSON",
      comment: "Tab of the right panel: the JSON of the selection.",
    }),
    tabCode: i18n._({
      id: "panelTabs.tabCode",
      message: "Code",
      comment: "Tab of the right panel: the functions the selection uses.",
    }),
    tabLocalisation: i18n._({
      id: "panelTabs.tabLocalisation",
      message: "Localisation",
      comment: "Tab of the right panel: the strings of the selection.",
    }),
    tabPerformance: i18n._({
      id: "panelTabs.tabPerformance",
      message: "Performance",
      comment: "Tab of the right panel: node count, cost and budget of the selection.",
    }),
    jsonOf: i18n._({
      id: "panelTabs.jsonOf",
      message: "JSON of the selection",
      comment: "Accessible name of the compact JSON of the selected nodes.",
    }),
    openJson: i18n._({
      id: "panelTabs.openJson",
      message: "Open in the JSON editor",
      comment: "Button of the JSON tab that opens the full-screen JSON editor.",
    }),
    codeNone: i18n._({
      id: "panelTabs.codeNone",
      message: "The selection calls no functions.",
      comment: "Shown in the Code tab when no selected node calls a function.",
    }),
    codeList: i18n._({
      id: "panelTabs.codeList",
      message: "Functions used by the selection",
      comment: "Accessible name of the list of functions the selection calls.",
    }),
    codeEdit: (name: string) =>
      i18n._({
        id: "panelTabs.codeEdit",
        message: "Edit {name} in the Rust editor",
        values: { name },
        comment: "Button in the Code tab that opens the Rust editor for the named function.",
      }),
    codeTest: (name: string) =>
      i18n._({
        id: "panelTabs.codeTest",
        message: "Run the tests of {name}",
        values: { name },
        comment: "Button in the Code tab that runs the stored tests of the named function.",
      }),
    codeRunning: i18n._({
      id: "panelTabs.codeRunning",
      message: "Running tests…",
      comment: "Status in the Code tab while the tests of a function run.",
    }),
    codeResult: (passed: number, failed: number) =>
      i18n._({
        id: "panelTabs.codeResult",
        message: "{passed} passed, {failed} failed",
        values: { passed, failed },
        comment: "Status in the Code tab after the tests of a function ran.",
      }),
    codeFailed: i18n._({
      id: "panelTabs.codeFailed",
      message: "The test run failed.",
      comment: "Status in the Code tab when the tests of a function could not run.",
    }),
    codePlugin: i18n._({
      id: "panelTabs.codePlugin",
      message: "Functions of the plugin",
      comment: "Heading in the Code tab: the functions the plugin may call.",
    }),
    l10nNone: i18n._({
      id: "panelTabs.l10nNone",
      message: "The selection shows no translatable text.",
      comment: "Shown in the Localisation tab when the selection has no translatable strings.",
    }),
    l10nList: i18n._({
      id: "panelTabs.l10nList",
      message: "Strings of the selection",
      comment: "Accessible name of the list of strings of the selection.",
    }),
    l10nLiteral: i18n._({
      id: "panelTabs.l10nLiteral",
      message: "Not translated",
      comment: "Tag beside a string of the Localisation tab that is a literal and has no translation key.",
    }),
    l10nOpen: i18n._({
      id: "panelTabs.l10nOpen",
      message: "Open the localisation table",
      comment: "Button of the Localisation tab that opens the localisation table.",
    }),
    perfNodes: i18n._({
      id: "panelTabs.perfNodes",
      message: "Nodes",
      comment: "Label of the number of nodes in the selection.",
    }),
    perfPage: i18n._({
      id: "panelTabs.perfPage",
      message: "Nodes in the page",
      comment: "Label of the number of nodes of the whole page.",
    }),
    perfBudget: i18n._({
      id: "panelTabs.perfBudget",
      message: "Node budget",
      comment: "Accessible name of the meter that shows the page's nodes against its budget.",
    }),
    perfBudgetText: (count: number, limit: number) =>
      i18n._({
        id: "panelTabs.perfBudgetText",
        message: "{count} of {limit} nodes",
        values: { count, limit },
        comment: "Reading of the node budget meter: nodes used of the allowed nodes.",
      }),
    perfCost: i18n._({
      id: "panelTabs.perfCost",
      message: "Estimated cost",
      comment: "Label of the share of the page's node budget the selection uses.",
    }),
    perfWithin: i18n._({
      id: "panelTabs.perfWithin",
      message: "Within budget",
      comment: "Status word: the page is within its node budget.",
    }),
    perfWarning: i18n._({
      id: "panelTabs.perfWarning",
      message: "Over the warning threshold",
      comment: "Status word: the page has more nodes than the warning threshold.",
    }),
    perfOver: i18n._({
      id: "panelTabs.perfOver",
      message: "Over budget",
      comment: "Status word: the page has more nodes than the budget allows.",
    }),
    perfBundle: i18n._({
      id: "panelTabs.perfBundle",
      message: "Bundle size",
      comment: "Label of the plugin's compiled bundle size.",
    }),
    perfScreens: i18n._({
      id: "panelTabs.perfScreens",
      message: "Screens",
      comment: "Label of the number of screens of the plugin.",
    }),
    perfVersions: i18n._({
      id: "panelTabs.perfVersions",
      message: "Versions",
      comment: "Label of the number of published versions of the plugin.",
    }),
    perfErrors: i18n._({
      id: "panelTabs.perfErrors",
      message: "Validation errors",
      comment: "Label of the number of validation errors of the plugin.",
    }),
    perfFunctions: i18n._({
      id: "panelTabs.perfFunctions",
      message: "Functions",
      comment: "Label of the number of functions the plugin uses.",
    }),
    perfUnknown: i18n._({
      id: "panelTabs.perfUnknown",
      message: "Not available",
      comment: "Shown for a plugin number the server did not return.",
    }),
    perfPlugin: i18n._({
      id: "panelTabs.perfPlugin",
      message: "Plugin",
      comment: "Heading of the section with the plugin's summary numbers.",
    }),
    perfSelection: i18n._({
      id: "panelTabs.perfSelection",
      message: "Selection",
      comment: "Heading of the section with the selection's numbers.",
    }),
  };
}
