// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the localisation area: the grid, the heatmap and the add, import and export dialogs (S8.3, STU-111, I18N-009). */
export function localisationAreaMessages(i18n: I18n) {
  return {
    gridTitle: i18n._({
      id: "l10n.area.gridTitle",
      message: "Localisation",
      comment: "Heading of the page that lists translation keys by locale.",
    }),
    gridCaption: i18n._({
      id: "l10n.area.gridCaption",
      message: "Translation keys by locale. Filter, search and change the status of many messages at once.",
      comment: "Caption under the heading of the localisation grid.",
    }),
    heatmapLink: i18n._({
      id: "l10n.area.heatmapLink",
      message: "Heatmap",
      comment: "Link from the localisation grid to the heatmap of missing translations.",
    }),
    gridLink: i18n._({
      id: "l10n.area.gridLink",
      message: "Grid",
      comment: "Link from the heatmap back to the localisation grid.",
    }),
    heatTitle: i18n._({
      id: "l10n.area.heatTitle",
      message: "Missing translations",
      comment: "Heading of the heatmap page: translations missing per plugin and locale.",
    }),
    heatCaption: i18n._({
      id: "l10n.area.heatCaption",
      message: "For each plugin, how many of the translation keys it uses a locale still lacks.",
      comment: "Caption under the heading of the heatmap page.",
    }),
    heatTable: i18n._({
      id: "l10n.area.heatTable",
      message: "Missing translations by plugin and locale",
      comment: "Accessible name of the heatmap table.",
    }),
    heatPlugin: i18n._({
      id: "l10n.area.heatPlugin",
      message: "Plugin",
      comment: "Column heading: the plugin of a heatmap row.",
    }),
    heatCell: (v: { missing: string | number; total: string | number }) =>
      i18n._({
        id: "l10n.area.heatCell",
        message: "{missing} of {total} missing",
        values: v,
        comment: "Text of one heatmap cell: how many of a plugin's keys a locale lacks.",
      }),
    heatEmpty: i18n._({
      id: "l10n.area.heatEmpty",
      message: "No plugin uses translation keys yet.",
      comment: "Shown on the heatmap when no plugin references a translation key.",
    }),
    pluginFilter: i18n._({
      id: "l10n.area.pluginFilter",
      message: "Plugin",
      comment: "Label of the list that limits the grid to the keys one plugin uses.",
    }),
    allPlugins: i18n._({
      id: "l10n.area.allPlugins",
      message: "All keys",
      comment: "Option: show every translation key of the app.",
    }),
    localeFilter: i18n._({
      id: "l10n.area.localeFilter",
      message: "Locale",
      comment: "Label of the list that chooses the locale whose status and messages are edited.",
    }),
    statusFilter: i18n._({
      id: "l10n.area.statusFilter",
      message: "Status",
      comment: "Label of the list that filters the grid by the status of the messages.",
    }),
    statusAll: i18n._({
      id: "l10n.area.statusAll",
      message: "Any status",
      comment: "Option: do not filter by status.",
    }),
    statusMissing: i18n._({
      id: "l10n.area.statusMissing",
      message: "Missing",
      comment: "Status: the locale has no message for the key.",
    }),
    statusNew: i18n._({
      id: "l10n.area.statusNew",
      message: "New",
      comment: "Status: the message was copied from the source and is not translated yet.",
    }),
    statusTranslated: i18n._({
      id: "l10n.area.statusTranslated",
      message: "Needs review",
      comment: "Status: translated and waiting for a review.",
    }),
    statusReviewed: i18n._({
      id: "l10n.area.statusReviewed",
      message: "Reviewed",
      comment: "Status: a reviewer checked the message.",
    }),
    statusApproved: i18n._({
      id: "l10n.area.statusApproved",
      message: "Approved",
      comment: "Status: the message is approved for release.",
    }),
    search: i18n._({
      id: "l10n.area.search",
      message: "Search keys and texts",
      comment: "Label of the search field of the localisation grid.",
    }),
    gridTable: i18n._({
      id: "l10n.area.gridTable",
      message: "Translation keys by locale",
      comment: "Accessible name of the localisation grid.",
    }),
    sourceColumn: i18n._({
      id: "l10n.area.sourceColumn",
      message: "Source",
      comment: "Column heading: the message of the default locale.",
    }),
    targetColumn: (v: { locale: string | number }) =>
      i18n._({
        id: "l10n.area.targetColumn",
        message: "Translation ({locale})",
        values: v,
        comment: "Column heading: the message of the locale being edited.",
      }),
    statusColumn: i18n._({
      id: "l10n.area.statusColumn",
      message: "Status",
      comment: "Column heading: the status of the message.",
    }),
    contextColumn: i18n._({
      id: "l10n.area.contextColumn",
      message: "Context",
      comment: "Column heading: the screenshot context of a key.",
    }),
    selectRow: (v: { key: string | number }) =>
      i18n._({
        id: "l10n.area.selectRow",
        message: "Select {key}",
        values: v,
        comment: "Accessible name of the checkbox that selects one key for a bulk action.",
      }),
    selectAll: i18n._({
      id: "l10n.area.selectAll",
      message: "Select all shown keys",
      comment: "Accessible name of the checkbox that selects every key the grid shows.",
    }),
    context: (v: { screens: string | number }) =>
      i18n._({
        id: "l10n.area.context",
        message: "Screenshot: {screens}",
        values: v,
        comment: "Text beside a key that has a screenshot captured as translation context; lists the screens.",
      }),
    noContext: i18n._({
      id: "l10n.area.noContext",
      message: "No screenshot",
      comment: "Text beside a key without a screenshot context.",
    }),
    selected: (v: { count: string | number }) =>
      i18n._({
        id: "l10n.area.selected",
        message: "{count} selected",
        values: v,
        comment: "Count of keys selected for a bulk action.",
      }),
    bulkApprove: i18n._({
      id: "l10n.area.bulkApprove",
      message: "Approve",
      comment: "Bulk action: approve the selected messages.",
    }),
    bulkReview: i18n._({
      id: "l10n.area.bulkReview",
      message: "Mark as needs review",
      comment: "Bulk action: mark the selected messages as waiting for review.",
    }),
    bulkCopy: i18n._({
      id: "l10n.area.bulkCopy",
      message: "Copy from source",
      comment: "Bulk action: copy the source text into the selected, untranslated messages.",
    }),
    bulkTitle: (v: { count: string | number }) =>
      i18n._({
        id: "l10n.area.bulkTitle",
        message: "Apply this to {count} messages?",
        values: v,
        comment: "Title of the confirmation dialog of a bulk action.",
      }),
    bulkApproveText: (v: { locale: string | number }) =>
      i18n._({
        id: "l10n.area.bulkApproveText",
        message: "The selected messages of {locale} become approved.",
        values: v,
        comment: "Description in the confirmation of the approve action.",
      }),
    bulkReviewText: (v: { locale: string | number }) =>
      i18n._({
        id: "l10n.area.bulkReviewText",
        message: "The selected messages of {locale} are marked as needing review.",
        values: v,
        comment: "Description in the confirmation of the needs-review action.",
      }),
    bulkCopyText: (v: { locale: string | number }) =>
      i18n._({
        id: "l10n.area.bulkCopyText",
        message: "The source text replaces the messages of {locale} that are empty, and they become new.",
        values: v,
        comment: "Description in the confirmation of the copy-from-source action.",
      }),
    bulkConfirm: i18n._({
      id: "l10n.area.bulkConfirm",
      message: "Apply",
      comment: "Button that confirms a bulk action.",
    }),
    bulkCancel: i18n._({
      id: "l10n.area.bulkCancel",
      message: "Cancel",
      comment: "Button that cancels a bulk action.",
    }),
    readOnlyGrid: i18n._({
      id: "l10n.area.readOnlyGrid",
      message:
        "You can read the translations. Changing them needs the translations.edit permission and the lock of the locale.",
      comment: "Shown when the person cannot edit the messages.",
    }),
    noApprove: i18n._({
      id: "l10n.area.noApprove",
      message: "Approving needs the translations.approve permission.",
      comment: "Shown when the person may edit but not approve translations.",
    }),
    emptyGrid: i18n._({
      id: "l10n.area.emptyGrid",
      message: "No translation key matches.",
      comment: "Shown when the filters leave no translation key.",
    }),
    addLocale: i18n._({
      id: "l10n.area.addLocale",
      message: "Add a locale",
      comment: "Button that opens the dialog that adds a locale to the app.",
    }),
    addLocaleTitle: i18n._({
      id: "l10n.area.addLocaleTitle",
      message: "Add a locale",
      comment: "Title of the dialog that adds a locale.",
    }),
    localeCode: i18n._({
      id: "l10n.area.localeCode",
      message: "Locale code",
      comment: "Label of the field for a locale code such as de or pt-BR.",
    }),
    localeInvalid: i18n._({
      id: "l10n.area.localeInvalid",
      message: "Enter a locale code such as de or pt-BR.",
      comment: "Error: the locale code is not valid.",
    }),
    localeTaken: i18n._({
      id: "l10n.area.localeTaken",
      message: "The app already supports this locale.",
      comment: "Error: the locale is already supported.",
    }),
    addLocaleConfirm: i18n._({
      id: "l10n.area.addLocaleConfirm",
      message: "Add",
      comment: "Button that adds the locale.",
    }),
    importOpen: i18n._({
      id: "l10n.area.importOpen",
      message: "Import translations",
      comment: "Button that opens the import dialog.",
    }),
    importTitle: i18n._({
      id: "l10n.area.importTitle",
      message: "Import translations",
      comment: "Title of the import dialog.",
    }),
    importFormat: i18n._({
      id: "l10n.area.importFormat",
      message: "Format",
      comment: "Label of the list that chooses the format of an import or export.",
    }),
    importFile: i18n._({
      id: "l10n.area.importFile",
      message: "File",
      comment: "Label of the file chooser of the import dialog.",
    }),
    importInto: (v: { locale: string | number }) =>
      i18n._({
        id: "l10n.area.importInto",
        message: "The messages are imported into {locale}.",
        values: v,
        comment: "Tells which locale an import changes.",
      }),
    importRead: i18n._({
      id: "l10n.area.importRead",
      message: "Read the file",
      comment: "Button that reads the chosen file.",
    }),
    importTooLarge: i18n._({
      id: "l10n.area.importTooLarge",
      message: "The file is larger than the import limit.",
      comment: "Error: the import file is too large.",
    }),
    importMalformed: i18n._({
      id: "l10n.area.importMalformed",
      message: "The file is not a valid file of this format.",
      comment: "Error: the import file cannot be parsed.",
    }),
    importEmpty: i18n._({
      id: "l10n.area.importEmpty",
      message: "The file holds no translations.",
      comment: "Error: the import file has no entries.",
    }),
    importPreview: (v: { matched: string | number; unknown: string | number }) =>
      i18n._({
        id: "l10n.area.importPreview",
        message:
          "{matched} messages will be imported; {unknown} keys of the file are not keys of this app and are skipped.",
        values: v,
        comment: "Summary of a parsed import file.",
      }),
    importApply: i18n._({
      id: "l10n.area.importApply",
      message: "Import",
      comment: "Button that applies the import.",
    }),
    exportOpen: i18n._({
      id: "l10n.area.exportOpen",
      message: "Export translations",
      comment: "Button that opens the export dialog.",
    }),
    exportTitle: i18n._({
      id: "l10n.area.exportTitle",
      message: "Export translations",
      comment: "Title of the export dialog.",
    }),
    exportDownload: i18n._({
      id: "l10n.area.exportDownload",
      message: "Download",
      comment: "Button that downloads the export file.",
    }),
    close: i18n._({
      id: "l10n.area.close",
      message: "Close",
      comment: "Button that closes a dialog.",
    }),
    navLabel: i18n._({
      id: "l10n.area.navLabel",
      message: "Localisation pages",
      comment: "Accessible name of the navigation between the grid and the heatmap.",
    }),
    loading: i18n._({
      id: "l10n.area.loading",
      message: "Loading translations",
      comment: "Shown while the translations load.",
    }),
  };
}
