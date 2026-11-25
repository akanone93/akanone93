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
    colActions: i18n._({
      id: "design.colActions",
      message: "Actions",
      comment: "Heading of the tokens table's column of edit and remove buttons.",
    }),
    addTokenHeading: i18n._({
      id: "design.addTokenHeading",
      message: "Add a token",
      comment: "Heading of the card with the fields that add a design token.",
    }),
    exportButton: i18n._({
      id: "design.exportButton",
      message: "Export tokens",
      comment: "Button that downloads the theme's tokens as a DTCG JSON file.",
    }),
    pickColour: (field: string) =>
      i18n._({
        id: "design.pickColour",
        message: "Pick a colour for {field}",
        values: { field },
        comment: "Accessible name of the colour picker swatch beside a colour field; field is the field's label.",
      }),
    brandDarkValue: i18n._({
      id: "design.brandDarkValue",
      message: "Value in dark mode",
      comment: "Label of the optional dark-mode value of a brand overlay's override.",
    }),
    brandDarkHint: i18n._({
      id: "design.brandDarkHint",
      message: "Optional. Without it, dark mode uses the value above.",
      comment: "Hint under the dark-mode value of a brand override.",
    }),
    brandAdd: i18n._({
      id: "design.brandAdd",
      message: "Add an override",
      comment: "Button on a brand overlay's card that adds the value of one more token for the brand.",
    }),
    brandAddTitle: (brand: string) =>
      i18n._({
        id: "design.brandAddTitle",
        message: "Add an override to {brand}",
        values: { brand },
        comment: "Title of the dialog that adds a token's value to a brand overlay.",
      }),
    brandRemove: (brand: string) =>
      i18n._({
        id: "design.brandRemove",
        message: "Remove the {brand} overlay",
        values: { brand },
        comment: "Accessible name of the button that removes a brand overlay.",
      }),
    brandRemoveTitle: (brand: string) =>
      i18n._({
        id: "design.brandRemoveTitle",
        message: "Remove the {brand} overlay?",
        values: { brand },
        comment: "Title of the confirmation before a brand overlay is removed.",
      }),
    brandRemoveText: i18n._({
      id: "design.brandRemoveText",
      message:
        "Its overrides are removed with it, and a host that selects this brand shows the base tokens. Undo brings it back.",
      comment: "Body of the confirmation before a brand overlay is removed.",
    }),
    brandRemoveConfirm: i18n._({
      id: "design.brandRemoveConfirm",
      message: "Remove overlay",
      comment: "Confirm button that removes a brand overlay.",
    }),
    cancel: i18n._({ id: "design.cancel", message: "Cancel", comment: "Cancel button of a confirmation." }),
    searchTokens: i18n._({
      id: "design.searchTokens",
      message: "Search tokens",
      comment: "Placeholder and accessible name of the field that filters the tokens table by path or type.",
    }),
    noTokenMatches: i18n._({
      id: "design.noTokenMatches",
      message: "No token matches the search.",
      comment: "Shown when the tokens search keeps no token.",
    }),
    textSample: i18n._({
      id: "design.textSample",
      message: "Aa",
      comment: "A short sample of text drawn on a colour to show how it reads; use two letters of your script.",
    }),
    contrastPasses: (ratio: string) =>
      i18n._({
        id: "design.contrastPasses",
        message: "{ratio}:1 AA",
        values: { ratio },
        comment: "On a colour tile: the contrast of its text, which meets WCAG AA for normal text.",
      }),
    contrastFails: (ratio: string) =>
      i18n._({
        id: "design.contrastFails",
        message: "{ratio}:1 below AA",
        values: { ratio },
        comment: "On a colour tile: the contrast of its text, which is below WCAG AA (4.5:1) for normal text.",
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
    importFile: i18n._({
      id: "design.importFile",
      message: "Choose a tokens file",
      comment: "Title of the zone that takes a DTCG JSON file to import, by drop or the system picker.",
    }),
    importFileHint: i18n._({
      id: "design.importFileHint",
      message: "A .json file, or paste its content below.",
      comment: "Hint under the zone that takes a DTCG JSON file.",
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
    importSummary: (added: number, changed: number, same: number, skipped: number) =>
      i18n._({
        id: "design.importSummary",
        message: "New: {added} · Changed: {changed} · Unchanged: {same} · Skipped: {skipped}",
        values: { added, changed, same, skipped },
        comment:
          "Result of reading a DTCG file against the theme: tokens the theme lacks, tokens with another value or type, tokens it already has as they are, and tokens left out.",
      }),
    importNothing: i18n._({
      id: "design.importNothing",
      message: "The theme already has every token in the file as it is.",
      comment: "Shown after reading a DTCG file whose tokens would change nothing.",
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
        message: "{brand}: {n, plural, one {# override} other {# overrides}}",
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
    hostFontsTitle: i18n._({
      id: "design.hostFontsTitle",
      message: "Host app fonts",
      comment: "Heading of the list of font files the host app bundles.",
    }),
    hostFontsCaption: i18n._({
      id: "design.hostFontsCaption",
      message:
        "Font files your host app bundles and plugins use by name. Upload each file as an asset, then list it here so the canvas lays text out with the same file.",
      comment: "Caption under the host fonts heading.",
    }),
    hostFontFamily: i18n._({
      id: "design.hostFontFamily",
      message: "Font family",
      comment: "Label of the family name a host font is used by.",
    }),
    hostFontAsset: i18n._({
      id: "design.hostFontAsset",
      message: "Font file",
      comment: "Label of the uploaded font asset a host font is read from.",
    }),
    hostFontAdd: i18n._({ id: "design.hostFontAdd", message: "Add font", comment: "Button that lists a host font." }),
    hostFontRemove: (family: string) =>
      i18n._({
        id: "design.hostFontRemove",
        message: "Remove {family}",
        values: { family },
        comment: "Accessible name of the button that removes a host font from the list.",
      }),
    hostFontsNone: i18n._({
      id: "design.hostFontsNone",
      message: "No host fonts are listed.",
      comment: "Shown when the app lists no host fonts.",
    }),
    hostFontsNoAssets: i18n._({
      id: "design.hostFontsNoAssets",
      message: "Upload a TrueType or OpenType file in the Assets tab to list it here.",
      comment: "Shown when the app has no font asset to list.",
    }),
    hostFontsSaveFailed: i18n._({
      id: "design.hostFontsSaveFailed",
      message: "The font list could not be saved.",
      comment: "Error shown when saving the host fonts fails.",
    }),
    hostFontsNoPermission: i18n._({
      id: "design.hostFontsNoPermission",
      message: "You need permission to manage this app to change the list.",
      comment: "Why the host font controls are disabled.",
    }),
    scriptEthiopic: i18n._({ id: "design.scriptEthiopic", message: "Ethiopic", comment: "Name of a script." }),
    familiesLabel: (script: string) =>
      i18n._({
        id: "design.familiesLabel",
        message: "Font families for {script}, first choice first, separated by commas",
        values: { script },
        comment: "Label of the font family list of a script.",
      }),
    noFonts: (script: string) =>
      i18n._({
        id: "design.noFonts",
        message: "{script} text uses the device's fonts. Name a family to choose one.",
        values: { script },
        comment: "Shown under a script's font field while it lists no family.",
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
        message: "{script}: {n, plural, one {# font} other {# fonts}}",
        values: { script, n },
        comment: "A script with its number of fonts.",
      }),
    iconsTitle: i18n._({ id: "design.iconsTitle", message: "Icons", comment: "Heading of the icons page." }),
    iconsCaption: i18n._({
      id: "design.iconsCaption",
      message: "The app's own icons, and the built-in sets.",
      comment: "Caption under the icons heading.",
    }),
    customIconsTitle: i18n._({
      id: "design.customIconsTitle",
      message: "Custom icons",
      comment: "Heading of the app's own icons, its SVG assets.",
    }),
    customIconsNote: i18n._({
      id: "design.customIconsNote",
      message:
        "A custom icon is an SVG asset, shown with an Image: drag it from the Assets panel onto a screen, or pick it as an image's source. Give the image a colour to tint a one-colour icon. Devices draw it from the compiled vector form.",
      comment: "How custom icons are used. Assets panel and Image are names of Studio panels and widgets.",
    }),
    uploadIcons: i18n._({
      id: "design.uploadIcons",
      message: "Upload SVG icons",
      comment: "Label of the drop zone that uploads SVG files as custom icons.",
    }),
    uploadIconsHint: i18n._({
      id: "design.uploadIconsHint",
      message: "Drop SVG files here, or click to choose them. They are stored under icons/.",
      comment: "Hint of the icon drop zone. icons/ is a folder name: keep it.",
    }),
    uploadingIcons: (n: number) =>
      i18n._({
        id: "design.uploadingIcons",
        message: "{n, plural, one {Uploading # icon…} other {Uploading # icons…}}",
        values: { n },
        comment: "Shown while icons upload.",
      }),
    notSvg: (name: string) =>
      i18n._({
        id: "design.notSvg",
        message: "{name} is not an SVG file. Upload other files in the Assets panel.",
        values: { name },
        comment: "A file dropped on the icon upload that is not an SVG.",
      }),
    noCustomIcons: i18n._({
      id: "design.noCustomIcons",
      message: "No custom icons yet.",
      comment: "Empty state of the custom icons.",
    }),
    iconsUnavailable: i18n._({
      id: "design.iconsUnavailable",
      message: "The app's assets could not be listed.",
      comment: "Shown when the asset list fails to load.",
    }),
    iconsAtRoot: i18n._({
      id: "design.iconsAtRoot",
      message: "Outside a folder",
      comment: "Heading of the icons whose name has no folder.",
    }),
    iconPending: i18n._({
      id: "design.iconPending",
      message: "Compiling…",
      comment: "The server is still compiling the SVG for devices.",
    }),
    iconFailed: i18n._({
      id: "design.iconFailed",
      message: "Devices cannot draw it",
      comment: "The server could not compile the SVG for devices; the reason is in the tooltip.",
    }),
    builtInIconsTitle: i18n._({
      id: "design.builtInIconsTitle",
      message: "Built-in sets",
      comment: "Heading of the Material Symbols and Cupertino icon sets.",
    }),
    builtInIconsNote: i18n._({
      id: "design.builtInIconsNote",
      message:
        "Material Symbols and Cupertino icons are chosen by name in an icon property, such as an Icon's icon. A plugin carries only the icons it uses.",
      comment: "How the built-in icon sets are used. Material Symbols and Cupertino are product names: keep them.",
    }),
    unreadIconTokens: i18n._({
      id: "design.unreadIconTokens",
      message: "Icon tokens nothing reads",
      comment: "Heading of icon design tokens an earlier Studio saved, which no widget uses.",
    }),
    unreadIconTokensNote: i18n._({
      id: "design.unreadIconTokensNote",
      message: "An earlier version of this page saved icons as design tokens. No widget reads them; remove them.",
      comment: "Explains the unused icon tokens.",
    }),
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
