// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the Design system area (STU-112, THM-004). */
export function designMessages(i18n: I18n) {
  return {
    areaNav: i18n._({
      id: "design.areaNav",
      message: "Design system sections",
      comment: "Accessible name of the navigation between the pages of the Design system area.",
    }),
    tokensLink: i18n._({ id: "design.tokensLink", message: "Tokens", comment: "Link to the tokens page." }),
    themesLink: i18n._({ id: "design.themesLink", message: "Themes", comment: "Link to the themes page." }),
    typographyLink: i18n._({
      id: "design.typographyLink",
      message: "Typography",
      comment: "Link to the typography page.",
    }),
    iconsLink: i18n._({ id: "design.iconsLink", message: "Icons", comment: "Link to the icon sets page." }),
    libraryLink: i18n._({
      id: "design.libraryLink",
      message: "Component library",
      comment: "Link to the component library page.",
    }),
    readOnly: i18n._({
      id: "design.readOnly",
      message: "Read-only: you cannot edit the theme now.",
      comment: "Shown when the app lock is not held.",
    }),
    saveRefused: i18n._({
      id: "design.saveRefused",
      message: "The server refused this change. It was undone.",
      comment: "Shown when autosave of a theme edit is refused by the server and the edit is rolled back.",
    }),
    noTheme: i18n._({
      id: "design.noTheme",
      message: "This app has no theme document yet.",
      comment: "Shown when theme.json is missing.",
    }),
    close: i18n._({ id: "design.close", message: "Close", comment: "Close button of a dialog." }),
    tokensTitle: i18n._({ id: "design.tokensTitle", message: "Design tokens", comment: "Heading of the tokens page." }),
    tokensCaption: i18n._({
      id: "design.tokensCaption",
      message: "The colours, sizes and other values of the app, with their light, dark and high-contrast values.",
      comment: "Caption under the tokens heading.",
    }),
    tokensTable: i18n._({
      id: "design.tokensTable",
      message: "Design tokens",
      comment: "Accessible name of the tokens table.",
    }),
    noTokens: i18n._({ id: "design.noTokens", message: "No design tokens yet.", comment: "Empty state." }),
    colToken: i18n._({ id: "design.colToken", message: "Token", comment: "Column heading." }),
    colLight: i18n._({
      id: "design.colLight",
      message: "Light",
      comment: "Column heading and name of the light mode.",
    }),
    colDark: i18n._({ id: "design.colDark", message: "Dark", comment: "Column heading and name of the dark mode." }),
    colContrast: i18n._({
      id: "design.colContrast",
      message: "High contrast",
      comment: "Column heading and name of the high-contrast mode.",
    }),
    editToken: (token: string) =>
      i18n._({
        id: "design.editToken",
        message: "Edit {token}",
        values: { token },
        comment: "Button that opens the token editor; {token} is its path.",
      }),
    editTitle: (token: string) =>
      i18n._({
        id: "design.editTitle",
        message: "Edit token {token}",
        values: { token },
        comment: "Title of dialog D57.",
      }),
    valueOf: (mode: string) =>
      i18n._({
        id: "design.valueOf",
        message: "{mode} value",
        values: { mode },
        comment: "Label of a token value field; {mode} is the mode.",
      }),
    removeToken: (token: string) =>
      i18n._({
        id: "design.removeToken",
        message: "Remove {token}",
        values: { token },
        comment: "Button that removes a token.",
      }),
    newName: i18n._({
      id: "design.newName",
      message: "New token path",
      comment: "Label of the field for the path of a new token.",
    }),
    newValue: i18n._({
      id: "design.newValue",
      message: "Value",
      comment: "Label of the field for the value of a new token.",
    }),
    newType: i18n._({ id: "design.newType", message: "Type", comment: "Label of the token type picker." }),
    addToken: i18n._({ id: "design.addToken", message: "Add token", comment: "Button." }),
    importButton: i18n._({
      id: "design.importButton",
      message: "Import tokens",
      comment: "Button that opens dialog D58.",
    }),
    importTitle: i18n._({ id: "design.importTitle", message: "Import design tokens", comment: "Title of dialog D58." }),
    importLabel: i18n._({
      id: "design.importLabel",
      message: "Design tokens file (DTCG JSON, such as a Figma variables export)",
      comment: "Label of the field that takes the DTCG text.",
    }),
    importRead: i18n._({
      id: "design.importRead",
      message: "Read the file",
      comment: "Button that parses the DTCG text.",
    }),
    importApply: i18n._({
      id: "design.importApply",
      message: "Add to the theme",
      comment: "Button that merges the read tokens into the theme.",
    }),
    importTooLarge: i18n._({
      id: "design.importTooLarge",
      message: "The file is larger than the document size limit.",
      comment: "Error when the DTCG text is too large.",
    }),
    importNotJson: i18n._({
      id: "design.importNotJson",
      message: "The file is not valid JSON.",
      comment: "Error when the DTCG text does not parse.",
    }),
    importNotObject: i18n._({
      id: "design.importNotObject",
      message: "The file does not hold a token tree.",
      comment: "Error when the DTCG JSON is not an object.",
    }),
    importSummary: (imported: number, skipped: number) =>
      i18n._({
        id: "design.importSummary",
        message: "{imported} tokens can be added, {skipped} are skipped.",
        values: { imported, skipped },
        comment: "Result of reading a DTCG file.",
      }),
    importDone: i18n._({
      id: "design.importDone",
      message: "The tokens were added to the theme.",
      comment: "Confirmation after the import.",
    }),
    skippedHeading: i18n._({
      id: "design.skippedHeading",
      message: "Skipped",
      comment: "Heading of the list of skipped tokens.",
    }),
    skipName: i18n._({
      id: "design.skipName",
      message: "the name is not allowed",
      comment: "Reason a token was skipped.",
    }),
    skipType: i18n._({
      id: "design.skipType",
      message: "the type is missing or unknown",
      comment: "Reason a token was skipped.",
    }),
    skipValue: i18n._({
      id: "design.skipValue",
      message: "the value does not fit the type",
      comment: "Reason a token was skipped.",
    }),
    skipDepth: i18n._({
      id: "design.skipDepth",
      message: "it is nested too deeply",
      comment: "Reason a token was skipped.",
    }),
    themesTitle: i18n._({
      id: "design.themesTitle",
      message: "Themes and brand overlays",
      comment: "Heading of the themes page.",
    }),
    themesCaption: i18n._({
      id: "design.themesCaption",
      message: "Light, dark and high-contrast values, and brand overlays that replace tokens for one brand.",
      comment: "Caption under the themes heading.",
    }),
    modesHeading: i18n._({ id: "design.modesHeading", message: "Modes", comment: "Heading of the modes section." }),
    modeCount: (mode: string, n: number) =>
      i18n._({
        id: "design.modeCount",
        message: "{mode}: {n} colours",
        values: { mode, n },
        comment: "Number of colour tokens that have a value in a mode.",
      }),
    swatches: (mode: string) =>
      i18n._({
        id: "design.swatches",
        message: "Colours in {mode}",
        values: { mode },
        comment: "Accessible name of a swatch list.",
      }),
    brandsHeading: i18n._({
      id: "design.brandsHeading",
      message: "Brand overlays",
      comment: "Heading of the brand overlays section.",
    }),
    noBrands: i18n._({ id: "design.noBrands", message: "No brand overlays yet.", comment: "Empty state." }),
    brandOverrides: (brand: string, n: number) =>
      i18n._({
        id: "design.brandOverrides",
        message: "{brand}: {n} overrides",
        values: { brand, n },
        comment: "A brand overlay and the number of tokens it replaces.",
      }),
    createBrand: i18n._({
      id: "design.createBrand",
      message: "Create a brand overlay",
      comment: "Button that opens the brand overlay dialog.",
    }),
    createBrandTitle: i18n._({
      id: "design.createBrandTitle",
      message: "Create a brand overlay",
      comment: "Title of dialog D57 when it creates an overlay.",
    }),
    brandName: i18n._({ id: "design.brandName", message: "Brand name", comment: "Field label." }),
    brandToken: i18n._({
      id: "design.brandToken",
      message: "Token to replace",
      comment: "Picker of the base token a brand replaces.",
    }),
    brandValue: i18n._({ id: "design.brandValue", message: "Value for the brand", comment: "Field label." }),
    brandCreate: i18n._({ id: "design.brandCreate", message: "Create overlay", comment: "Button." }),
    typographyTitle: i18n._({
      id: "design.typographyTitle",
      message: "Typography per script",
      comment: "Heading of the typography page.",
    }),
    typographyCaption: i18n._({
      id: "design.typographyCaption",
      message: "One font family list per script, with fallbacks, so no text shows missing glyphs.",
      comment: "Caption under the typography heading.",
    }),
    scriptLatin: i18n._({ id: "design.scriptLatin", message: "Latin", comment: "Name of a script." }),
    scriptCyrillic: i18n._({ id: "design.scriptCyrillic", message: "Cyrillic", comment: "Name of a script." }),
    scriptArabic: i18n._({ id: "design.scriptArabic", message: "Arabic", comment: "Name of a script." }),
    scriptDevanagari: i18n._({ id: "design.scriptDevanagari", message: "Devanagari", comment: "Name of a script." }),
    scriptEthiopic: i18n._({ id: "design.scriptEthiopic", message: "Ethiopic", comment: "Name of a script." }),
    familiesLabel: (script: string) =>
      i18n._({
        id: "design.familiesLabel",
        message: "Font families for {script}, first choice first, separated by commas",
        values: { script },
        comment: "Label of the font family list of a script.",
      }),
    noFallback: (script: string) =>
      i18n._({
        id: "design.noFallback",
        message: "{script} has no fallback font.",
        values: { script },
        comment: "Warning when a script has fewer than two families.",
      }),
    fallbackOk: (script: string, n: number) =>
      i18n._({
        id: "design.fallbackOk",
        message: "{script}: {n} fonts",
        values: { script, n },
        comment: "A script with its number of fonts.",
      }),
    iconsTitle: i18n._({ id: "design.iconsTitle", message: "Icon sets", comment: "Heading of the icon sets page." }),
    iconsCaption: i18n._({
      id: "design.iconsCaption",
      message: "The icons each set provides to the app, by name.",
      comment: "Caption under the icon sets heading.",
    }),
    noIcons: i18n._({ id: "design.noIcons", message: "No icon sets yet.", comment: "Empty state." }),
    iconSet: i18n._({ id: "design.iconSet", message: "Icon set", comment: "Field label." }),
    iconName: i18n._({ id: "design.iconName", message: "Icon name", comment: "Field label." }),
    iconSource: i18n._({
      id: "design.iconSource",
      message: "Icon source (asset or code point)",
      comment: "Field label.",
    }),
    addIcon: i18n._({ id: "design.addIcon", message: "Add icon", comment: "Button." }),
    removeIcon: (icon: string) =>
      i18n._({ id: "design.removeIcon", message: "Remove icon {icon}", values: { icon }, comment: "Button." }),
    libraryTitle: i18n._({
      id: "design.libraryTitle",
      message: "Component library",
      comment: "Heading of the library page.",
    }),
    libraryCaption: i18n._({
      id: "design.libraryCaption",
      message: "The organisation's locked components and the version this app uses of each.",
      comment: "Caption under the library heading.",
    }),
    libraryTable: i18n._({
      id: "design.libraryTable",
      message: "Components of the library",
      comment: "Accessible name of the table.",
    }),
    libraryNone: i18n._({
      id: "design.libraryNone",
      message: "The library has no components.",
      comment: "Empty state.",
    }),
    libraryLoading: i18n._({ id: "design.libraryLoading", message: "Loading the library.", comment: "Status." }),
    libraryFailed: i18n._({
      id: "design.libraryFailed",
      message: "The library could not be loaded.",
      comment: "Error.",
    }),
    colComponent: i18n._({ id: "design.colComponent", message: "Component", comment: "Column heading." }),
    colAdopted: i18n._({ id: "design.colAdopted", message: "Version in this app", comment: "Column heading." }),
    notAdopted: i18n._({
      id: "design.notAdopted",
      message: "Not adopted",
      comment: "Cell value when the app has no version of the component.",
    }),
    adoptedVersion: (n: number) =>
      i18n._({
        id: "design.adoptedVersion",
        message: "Version {n}",
        values: { n },
        comment: "Cell value: the adopted version.",
      }),
    adopt: (name: string) =>
      i18n._({
        id: "design.adopt",
        message: "Adopt a version of {name}",
        values: { name },
        comment: "Button that opens dialog D32.",
      }),
  };
}

export type DesignMessages = ReturnType<typeof designMessages>;
