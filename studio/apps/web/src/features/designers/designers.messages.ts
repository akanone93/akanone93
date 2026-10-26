// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the data source, DB schema and theme designers (S5.5, STU-063). */
export function designerMessages(i18n: I18n) {
  return {
    apply: i18n._({
      id: "designers.apply",
      message: "Apply",
      comment: "Button that applies a designer's draft to the document.",
    }),
    applied: i18n._({
      id: "designers.applied",
      message: "Applied to the document.",
      comment: "Status after a designer's draft was applied.",
    }),
    nothingToApply: i18n._({
      id: "designers.nothingToApply",
      message: "Nothing to apply.",
      comment: "Status when the draft equals the document.",
    }),
    readOnly: i18n._({
      id: "designers.readOnly",
      message: "Someone else is editing, so this designer is read-only.",
      comment: "Shown when the editing lock is held by someone else.",
    }),
    checking: i18n._({
      id: "designers.checking",
      message: "Checking\u2026",
      comment: "Status while the validator checks the draft.",
    }),
    unavailable: i18n._({
      id: "designers.unavailable",
      message: "The validator is not available.",
      comment: "Status when the validator worker cannot be reached.",
    }),
    valid: i18n._({
      id: "designers.valid",
      message: "Valid. You can apply it.",
      comment: "Status when the draft passes validation.",
    }),
    invalid: i18n._({
      id: "designers.invalid",
      message: "The draft has errors; fix them to apply it.",
      comment: "Status when the draft fails validation.",
    }),
    problems: i18n._({
      id: "designers.problems",
      message: "Problems",
      comment: "Heading of the list of validation findings in a designer.",
    }),
    noProblems: i18n._({
      id: "designers.noProblems",
      message: "No problems.",
      comment: "Shown when validation found nothing.",
    }),
    name: i18n._({
      id: "designers.name",
      message: "Name",
      comment: "Label of a name field.",
    }),
    add: i18n._({
      id: "designers.add",
      message: "Add",
      comment: "Button that adds an entry to a list in a designer.",
    }),
    remove: i18n._({
      id: "designers.remove",
      message: "Remove",
      comment: "Button that removes an entry from a list in a designer.",
    }),
    dataOpen: i18n._({
      id: "designers.dataOpen",
      message: "Data sources",
      comment: "Button in the right panel that opens the data source designer.",
    }),
    dataTitle: i18n._({
      id: "designers.dataTitle",
      message: "Data source designer",
      comment: "Title of the full-screen data source designer.",
    }),
    dataClose: i18n._({
      id: "designers.dataClose",
      message: "Close the data source designer",
      comment: "Accessible name of the button that closes the data source designer.",
    }),
    dataList: i18n._({
      id: "designers.dataList",
      message: "Data sources of the plugin",
      comment: "Accessible name of the list of data sources.",
    }),
    dataNone: i18n._({
      id: "designers.dataNone",
      message: "This plugin has no data sources yet.",
      comment: "Shown when the plugin declares no data source.",
    }),
    dataAdd: i18n._({
      id: "designers.dataAdd",
      message: "Add data source",
      comment: "Button that adds an empty data source.",
    }),
    dataRemove: i18n._({
      id: "designers.dataRemove",
      message: "Remove data source",
      comment: "Button that removes the selected data source.",
    }),
    dataSelect: i18n._({
      id: "designers.dataSelect",
      message: "Select a data source to edit it.",
      comment: "Shown when no data source is selected.",
    }),
    dataKind: i18n._({
      id: "designers.dataKind",
      message: "Kind",
      comment: "Label of the list that chooses the kind of a data source (REST, GraphQL, \u2026).",
    }),
    dataType: i18n._({
      id: "designers.dataType",
      message: "Value type",
      comment: "Label of the field that holds the type of the value a data source returns.",
    }),
    dataMock: i18n._({
      id: "designers.dataMock",
      message: "Mock value (JSON)",
      comment: "Label of the text area that holds the design-time mock of a data source.",
    }),
    dataMockHelp: i18n._({
      id: "designers.dataMockHelp",
      message: "The mock is what the canvas shows while no real data is available.",
      comment: "Help text under the mock field of a data source.",
    }),
    dataMockInvalid: i18n._({
      id: "designers.dataMockInvalid",
      message: "The mock is not valid JSON.",
      comment: "Error under the mock field when it cannot be parsed.",
    }),
    dataImportHeading: i18n._({
      id: "designers.dataImportHeading",
      message: "Import a data source",
      comment: "Heading of the import section of the data source designer.",
    }),
    dataImportFormat: i18n._({
      id: "designers.dataImportFormat",
      message: "Source format",
      comment: "Label of the list that chooses OpenAPI or GraphQL for an import.",
    }),
    dataOpenApi: i18n._({
      id: "designers.dataOpenApi",
      message: "OpenAPI",
      comment: "Name of the OpenAPI format in the import list.",
    }),
    dataGraphQl: i18n._({
      id: "designers.dataGraphQl",
      message: "GraphQL",
      comment: "Name of the GraphQL format in the import list.",
    }),
    dataSourceText: i18n._({
      id: "designers.dataSourceText",
      message: "Source text: an OpenAPI document or a GraphQL schema",
      comment: "Label of the text area that holds the text to import.",
    }),
    dataOperationsText: i18n._({
      id: "designers.dataOperationsText",
      message: "GraphQL operations",
      comment: "Label of the text area that holds the GraphQL operations to import.",
    }),
    dataChooseFile: i18n._({
      id: "designers.dataChooseFile",
      message: "Read a file from this computer",
      comment: "Label of the control that fills the import text from a file the user chooses.",
    }),
    dataRunImport: i18n._({
      id: "designers.dataRunImport",
      message: "Read the source",
      comment: "Button that runs the import on the text.",
    }),
    dataImporting: i18n._({
      id: "designers.dataImporting",
      message: "Reading the source\u2026",
      comment: "Status while the importer runs.",
    }),
    dataImportFailed: i18n._({
      id: "designers.dataImportFailed",
      message: "The importer is not available.",
      comment: "Shown when the importer in the validator module cannot be reached.",
    }),
    dataImportNothing: i18n._({
      id: "designers.dataImportNothing",
      message: "Nothing could be imported from this source.",
      comment: "Shown when an import produced no data source.",
    }),
    dataImportReady: i18n._({
      id: "designers.dataImportReady",
      message: "The source was read. Review it, then add it to the plugin.",
      comment: "Status after a successful import.",
    }),
    dataAddImported: i18n._({
      id: "designers.dataAddImported",
      message: "Add the imported data sources",
      comment: "Button that adds the imported data sources and types to the draft.",
    }),
    dataVariablesNote: i18n._({
      id: "designers.dataVariablesNote",
      message: "The source also describes app-level variables; add them in the app document.",
      comment: "Note shown after an import that produced variables, which belong to the app and are not added here.",
    }),
    dbOpen: i18n._({
      id: "designers.dbOpen",
      message: "DB schema",
      comment: "Button in the right panel that opens the DB schema designer.",
    }),
    dbTitle: i18n._({
      id: "designers.dbTitle",
      message: "DB schema designer",
      comment: "Title of the full-screen DB schema designer.",
    }),
    dbClose: i18n._({
      id: "designers.dbClose",
      message: "Close the DB schema designer",
      comment: "Accessible name of the button that closes the DB schema designer.",
    }),
    dbList: i18n._({
      id: "designers.dbList",
      message: "Collections of the plugin",
      comment: "Accessible name of the list of collections.",
    }),
    dbNone: i18n._({
      id: "designers.dbNone",
      message: "This plugin has no collections yet.",
      comment: "Shown when the plugin declares no collection.",
    }),
    dbAdd: i18n._({
      id: "designers.dbAdd",
      message: "Add collection",
      comment: "Button that adds a collection.",
    }),
    dbRemove: i18n._({
      id: "designers.dbRemove",
      message: "Remove collection",
      comment: "Button that removes the selected collection and lists it as dropped.",
    }),
    dbSelect: i18n._({
      id: "designers.dbSelect",
      message: "Select a collection to edit it.",
      comment: "Shown when no collection is selected.",
    }),
    dbKey: i18n._({
      id: "designers.dbKey",
      message: "Key",
      comment: "Label of the field that holds a collection's key.",
    }),
    dbVersion: i18n._({
      id: "designers.dbVersion",
      message: "Version",
      comment: "Label of the field that holds a collection's schema version.",
    }),
    dbFields: i18n._({
      id: "designers.dbFields",
      message: "Fields",
      comment: "Heading of the list of fields of a collection.",
    }),
    dbAddField: i18n._({
      id: "designers.dbAddField",
      message: "Add field",
      comment: "Button that adds a field to a collection.",
    }),
    dbRemoveField: i18n._({
      id: "designers.dbRemoveField",
      message: "Remove field",
      comment: "Button that removes a field from a collection.",
    }),
    dbFieldName: i18n._({
      id: "designers.dbFieldName",
      message: "Field name",
      comment: "Label of the field that holds the name of a collection field.",
    }),
    dbFieldType: i18n._({
      id: "designers.dbFieldType",
      message: "Field type",
      comment: "Label of the field that holds the type of a collection field.",
    }),
    dbSensitive: i18n._({
      id: "designers.dbSensitive",
      message: "Sensitive",
      comment: "Label of the checkbox that marks a field as sensitive; it is then stored encrypted.",
    }),
    dbPrimaryKey: i18n._({
      id: "designers.dbPrimaryKey",
      message: "Primary key (field names, comma-separated)",
      comment: "Label of the field that holds the primary key of a collection.",
    }),
    dbPlans: i18n._({
      id: "designers.dbPlans",
      message: "Migration plans",
      comment: "Heading of the list of migration plans of a collection.",
    }),
    dbAddPlan: i18n._({
      id: "designers.dbAddPlan",
      message: "Add migration plan",
      comment: "Button that adds a migration plan.",
    }),
    dbRemovePlan: i18n._({
      id: "designers.dbRemovePlan",
      message: "Remove migration plan",
      comment: "Button that removes a migration plan.",
    }),
    dbPlanFrom: i18n._({
      id: "designers.dbPlanFrom",
      message: "From version",
      comment: "Label of the field that holds the version a migration plan starts from.",
    }),
    dbPlanDrop: i18n._({
      id: "designers.dbPlanDrop",
      message: "Drop fields (comma-separated)",
      comment: "Label of the field that lists the fields a plan drops.",
    }),
    dbPlanReset: i18n._({
      id: "designers.dbPlanReset",
      message: "Reset fields (comma-separated)",
      comment: "Label of the field that lists the fields a plan resets to an empty value.",
    }),
    dbPlanRename: i18n._({
      id: "designers.dbPlanRename",
      message: "Rename (new=old, comma-separated)",
      comment: "Label of the field that lists the renames of a plan.",
    }),
    dbPreview: i18n._({
      id: "designers.dbPreview",
      message: "Migration preview",
      comment: "Heading of the preview of what a change does to data on devices.",
    }),
    dbPreviewNoRelease: i18n._({
      id: "designers.dbPreviewNoRelease",
      message: "Nothing to compare with: this plugin has not been published yet.",
      comment: "Shown instead of the migration preview when there is no released version.",
    }),
    dbPreviewClean: i18n._({
      id: "designers.dbPreviewClean",
      message: "No migration problems: devices can update without losing data.",
      comment: "Shown when the migration preview has no finding.",
    }),
    dbPreviewChecking: i18n._({
      id: "designers.dbPreviewChecking",
      message: "Computing the migration\u2026",
      comment: "Status while the migration preview is computed.",
    }),
    dbPreviewUnavailable: i18n._({
      id: "designers.dbPreviewUnavailable",
      message: "The migration preview is not available.",
      comment: "Shown when the migration planner in the validator module cannot be reached.",
    }),
    themeOpen: i18n._({
      id: "designers.themeOpen",
      message: "Theme",
      comment: "Button in the right panel that opens the theme editor.",
    }),
    themeTitle: i18n._({
      id: "designers.themeTitle",
      message: "Theme editor",
      comment: "Title of the full-screen theme editor.",
    }),
    themeClose: i18n._({
      id: "designers.themeClose",
      message: "Close the theme editor",
      comment: "Accessible name of the button that closes the theme editor.",
    }),
    themeNone: i18n._({
      id: "designers.themeNone",
      message: "This app has no theme document.",
      comment: "Shown when the app has no theme.json.",
    }),
    themeLive: i18n._({
      id: "designers.themeLive",
      message: "Changes show on the canvas as you type.",
      comment: "Note that edits to the theme apply to the canvas at once.",
    }),
    themeTokens: i18n._({
      id: "designers.themeTokens",
      message: "Design tokens",
      comment: "Accessible name of the table of design tokens.",
    }),
    themeToken: i18n._({
      id: "designers.themeToken",
      message: "Token",
      comment: "Column heading: the path of a design token.",
    }),
    themeLight: i18n._({
      id: "designers.themeLight",
      message: "Light",
      comment: "Column heading and preview name of the light mode.",
    }),
    themeDark: i18n._({
      id: "designers.themeDark",
      message: "Dark",
      comment: "Column heading and preview name of the dark mode.",
    }),
    themeLightValue: (token: string) =>
      i18n._({
        id: "designers.themeLightValue",
        message: "Light value of {token}",
        values: { token },
        comment: "Accessible name of the field that holds a token's light value; {token} is its path.",
      }),
    themeDarkValue: (token: string) =>
      i18n._({
        id: "designers.themeDarkValue",
        message: "Dark value of {token}",
        values: { token },
        comment: "Accessible name of the field that holds a token's dark value; {token} is its path.",
      }),
    themeRemoveToken: (token: string) =>
      i18n._({
        id: "designers.themeRemoveToken",
        message: "Remove token {token}",
        values: { token },
        comment: "Accessible name of the button that removes a token; {token} is its path.",
      }),
    themeNewPath: i18n._({
      id: "designers.themeNewPath",
      message: "New token path, for example color.accent",
      comment: "Label of the field that holds the path of a token to add.",
    }),
    themeNewValue: i18n._({
      id: "designers.themeNewValue",
      message: "Value of the new token",
      comment: "Label of the field that holds the value of a token to add.",
    }),
    themeAddToken: i18n._({
      id: "designers.themeAddToken",
      message: "Add token",
      comment: "Button that adds a design token.",
    }),
    themePreview: i18n._({
      id: "designers.themePreview",
      message: "Colour preview",
      comment: "Heading of the swatches that show the colour tokens in both modes.",
    }),
  };
}

export type DesignerMessages = ReturnType<typeof designerMessages>;
