// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the Data area (STU-113, DAT-002, DAT-080). */
export function dataMessages(i18n: I18n) {
  return {
    loading: i18n._({
      id: "data.loading",
      message: "Loading the data area.",
      comment: "Status while the data area loads.",
    }),
    loadFailed: i18n._({
      id: "data.loadFailed",
      message: "The data area could not be loaded.",
      comment: "Error when the plugin documents fail to load.",
    }),
    pluginLabel: i18n._({ id: "data.pluginLabel", message: "Plugin", comment: "Label of the plugin picker." }),
    noPlugin: i18n._({
      id: "data.noPlugin",
      message: "This app has no plugin yet.",
      comment: "Shown when the app has no plugin.",
    }),
    envLabel: i18n._({
      id: "data.envLabel",
      message: "Environment",
      comment: "Label of the environment the page shows.",
    }),
    readOnly: i18n._({
      id: "data.readOnly",
      message: "Read-only: you cannot edit these documents now.",
      comment: "Shown when the editing lock is not held.",
    }),
    apply: i18n._({ id: "data.apply", message: "Apply", comment: "Button that applies the draft to the document." }),
    applied: i18n._({
      id: "data.applied",
      message: "Applied to the document.",
      comment: "Confirmation after applying.",
    }),
    valid: i18n._({ id: "data.valid", message: "The draft is valid.", comment: "Validation status." }),
    checking: i18n._({ id: "data.checking", message: "Checking the draft.", comment: "Validation status." }),
    unavailable: i18n._({
      id: "data.unavailable",
      message: "The validator is not available.",
      comment: "Validation status.",
    }),
    problems: i18n._({ id: "data.problems", message: "Problems", comment: "Heading of the findings list." }),
    listTitle: i18n._({
      id: "data.listTitle",
      message: "API definitions",
      comment: "Heading of the data sources page.",
    }),
    listCaption: i18n._({
      id: "data.listCaption",
      message: "The data sources of this plugin, their kinds and their mocks.",
      comment: "Caption under the data sources heading.",
    }),
    noSources: i18n._({ id: "data.noSources", message: "This plugin has no data sources.", comment: "Empty state." }),
    colName: i18n._({ id: "data.colName", message: "Name", comment: "Column heading." }),
    colKind: i18n._({ id: "data.colKind", message: "Kind", comment: "Column heading." }),
    colType: i18n._({ id: "data.colType", message: "Type", comment: "Column heading." }),
    colRoute: i18n._({ id: "data.colRoute", message: "Route", comment: "Column heading." }),
    colVersion: i18n._({ id: "data.colVersion", message: "Version", comment: "Column heading." }),
    colFields: i18n._({ id: "data.colFields", message: "Fields", comment: "Column heading." }),
    colIndexes: i18n._({ id: "data.colIndexes", message: "Indexes", comment: "Column heading." }),
    open: i18n._({ id: "data.open", message: "Open", comment: "Link to one data source or collection." }),
    importButton: i18n._({
      id: "data.importButton",
      message: "Import an API definition",
      comment: "Button that opens the import dialog.",
    }),
    collectionsLink: i18n._({
      id: "data.collectionsLink",
      message: "Collections",
      comment: "Link to the collections page.",
    }),
    importTitle: i18n._({
      id: "data.importTitle",
      message: "Import an API definition",
      comment: "Title of the import dialog.",
    }),
    importClose: i18n._({
      id: "data.importClose",
      message: "Close the import dialog",
      comment: "Label of the import dialog close button.",
    }),
    importStaged: i18n._({
      id: "data.importStaged",
      message: "Imported sources are ready to apply.",
      comment: "Shown when an import is staged.",
    }),
    importLocked: i18n._({
      id: "data.importLocked",
      message: "The app document cannot be edited now, so its variables cannot be added.",
      comment: "Shown when imported variables cannot be written.",
    }),
    sourceTitle: i18n._({
      id: "data.sourceTitle",
      message: "Data source",
      comment: "Heading of one data source page.",
    }),
    sourceCaption: i18n._({
      id: "data.sourceCaption",
      message: "Operations, schemas, per-environment URLs and the mock of this source.",
      comment: "Caption of one data source page.",
    }),
    notFound: i18n._({
      id: "data.notFound",
      message: "This data source does not exist.",
      comment: "Shown for an unknown source.",
    }),
    back: i18n._({ id: "data.back", message: "All API definitions", comment: "Link back to the list." }),
    name: i18n._({ id: "data.name", message: "Name", comment: "Field label." }),
    kind: i18n._({ id: "data.kind", message: "Kind", comment: "Field label." }),
    type: i18n._({ id: "data.type", message: "Type", comment: "Field label." }),
    route: i18n._({ id: "data.route", message: "Route", comment: "Field label of the data source route." }),
    routeDefault: i18n._({
      id: "data.routeDefault",
      message: "Default (Plux gateway)",
      comment: "Route option: absent means the gateway.",
    }),
    routePlux: i18n._({ id: "data.routePlux", message: "Through the Plux gateway", comment: "Route option." }),
    routeDirect: i18n._({ id: "data.routeDirect", message: "Straight to the origin", comment: "Route option." }),
    config: i18n._({ id: "data.config", message: "Configuration (JSON)", comment: "Field label." }),
    configInvalid: i18n._({
      id: "data.configInvalid",
      message: "The configuration must be a JSON object.",
      comment: "Field error.",
    }),
    transaction: i18n._({
      id: "data.transaction",
      message: "Transaction pointers (JSON)",
      comment: "Field label of the signed-transaction pointers.",
    }),
    transactionHelp: i18n._({
      id: "data.transactionHelp",
      message: "JSON Pointers to the amount, currency and payee; leave empty for none.",
      comment: "Help text.",
    }),
    transactionInvalid: i18n._({
      id: "data.transactionInvalid",
      message: "The transaction must be empty or a JSON object.",
      comment: "Field error.",
    }),
    operations: i18n._({ id: "data.operations", message: "Operations", comment: "Heading of the operations table." }),
    noOperations: i18n._({
      id: "data.noOperations",
      message: "The configuration names no operations.",
      comment: "Empty state.",
    }),
    schemas: i18n._({ id: "data.schemas", message: "Schemas", comment: "Heading of the schemas list." }),
    noSchemas: i18n._({
      id: "data.noSchemas",
      message: "No type of the plugin is used by this source.",
      comment: "Empty state.",
    }),
    urls: i18n._({ id: "data.urls", message: "URLs per environment", comment: "Heading of the URLs table." }),
    noUrls: i18n._({
      id: "data.noUrls",
      message: "The configuration names no per-environment URLs.",
      comment: "Empty state.",
    }),
    colEnvironment: i18n._({ id: "data.colEnvironment", message: "Environment", comment: "Column heading." }),
    colUrl: i18n._({ id: "data.colUrl", message: "URL", comment: "Column heading." }),
    colOperation: i18n._({ id: "data.colOperation", message: "Operation", comment: "Column heading." }),
    colMethod: i18n._({ id: "data.colMethod", message: "Method", comment: "Column heading." }),
    colPath: i18n._({ id: "data.colPath", message: "Path", comment: "Column heading." }),
    mock: i18n._({ id: "data.mock", message: "Mock", comment: "Heading of the mock section." }),
    editMock: i18n._({ id: "data.editMock", message: "Edit the mock", comment: "Button that opens the mock dialog." }),
    mockTitle: i18n._({ id: "data.mockTitle", message: "Edit the mock", comment: "Title of the mock dialog." }),
    mockClose: i18n._({
      id: "data.mockClose",
      message: "Close the mock dialog",
      comment: "Label of the mock dialog close button.",
    }),
    mockLabel: i18n._({ id: "data.mockLabel", message: "Mock value (JSON)", comment: "Field label." }),
    mockInvalid: i18n._({ id: "data.mockInvalid", message: "The mock is not valid JSON.", comment: "Field error." }),
    mockSave: i18n._({
      id: "data.mockSave",
      message: "Use this mock",
      comment: "Button that keeps the mock in the draft.",
    }),
    collectionsTitle: i18n._({
      id: "data.collectionsTitle",
      message: "Collections",
      comment: "Heading of the collections page.",
    }),
    collectionsCaption: i18n._({
      id: "data.collectionsCaption",
      message: "The collections of the on-device database of this plugin.",
      comment: "Caption of the collections page.",
    }),
    noCollections: i18n._({
      id: "data.noCollections",
      message: "This plugin has no collections.",
      comment: "Empty state.",
    }),
    sourcesLink: i18n._({
      id: "data.sourcesLink",
      message: "API definitions",
      comment: "Link to the data sources page.",
    }),
    collectionTitle: i18n._({
      id: "data.collectionTitle",
      message: "Collection",
      comment: "Heading of one collection page.",
    }),
    collectionCaption: i18n._({
      id: "data.collectionCaption",
      message: "The schema of this collection and what a migration from the last release does.",
      comment: "Caption.",
    }),
    collectionsBack: i18n._({
      id: "data.collectionsBack",
      message: "All collections",
      comment: "Link back to the list.",
    }),
    notFoundCollection: i18n._({
      id: "data.notFoundCollection",
      message: "This collection does not exist.",
      comment: "Shown for an unknown collection.",
    }),
    key: i18n._({ id: "data.key", message: "Key", comment: "Field label." }),
    version: i18n._({ id: "data.version", message: "Version", comment: "Field label." }),
    primaryKey: i18n._({ id: "data.primaryKey", message: "Primary key", comment: "Field label." }),
    indexes: i18n._({
      id: "data.indexes",
      message: "Indexes",
      comment: "Field label: one index per line, fields separated by commas.",
    }),
    indexesHelp: i18n._({
      id: "data.indexesHelp",
      message: "One index per line; separate the fields with commas.",
      comment: "Help text.",
    }),
    fields: i18n._({ id: "data.fields", message: "Fields", comment: "Heading of the fields table." }),
    sensitive: i18n._({ id: "data.sensitive", message: "sensitive", comment: "Marks a sensitive field." }),
    plans: i18n._({ id: "data.plans", message: "Migration plans", comment: "Heading of the migration plans list." }),
    noPlans: i18n._({ id: "data.noPlans", message: "No migration plans.", comment: "Empty state." }),
    planFrom: (from: number) =>
      i18n._({
        id: "data.planFrom",
        message: "From version {from}",
        values: { from },
        comment: "Describes one migration plan.",
      }),
    previewButton: i18n._({
      id: "data.previewButton",
      message: "Preview the migration",
      comment: "Button that opens the migration preview.",
    }),
    previewTitle: i18n._({
      id: "data.previewTitle",
      message: "Migration preview",
      comment: "Title of the migration preview dialog.",
    }),
    previewClose: i18n._({
      id: "data.previewClose",
      message: "Close the migration preview",
      comment: "Label of the dialog close button.",
    }),
  };
}

export type DataMessages = ReturnType<typeof dataMessages>;
