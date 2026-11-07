// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the device frames and preview switches (STU-057, STU-058). */
export function previewMessages(i18n: I18n) {
  return {
    switcher: i18n._({
      id: "preview.switcher",
      message: "Preview switches",
      comment: "Accessible name of the toolbar that changes how the canvas previews screens.",
    }),
    frame: i18n._({
      id: "preview.frame",
      message: "Device frame",
      comment: "Label of the select that picks the device frame of the preview.",
    }),
    customFrame: i18n._({
      id: "preview.customFrame",
      message: "Custom frame",
      comment: "Entry of the device frame select that opens the custom frame dialog; also the dialog title.",
    }),
    customFrameAdd: i18n._({
      id: "preview.customFrameAdd",
      message: "Use frame",
      comment: "Button of the custom frame dialog that applies the entered size.",
    }),
    width: i18n._({
      id: "preview.width",
      message: "Width (logical pixels)",
      comment: "Label of the width field of the custom frame dialog.",
    }),
    height: i18n._({
      id: "preview.height",
      message: "Height (logical pixels)",
      comment: "Label of the height field of the custom frame dialog.",
    }),
    density: i18n._({
      id: "preview.density",
      message: "Pixel density",
      comment: "Label of the pixel density field of the custom frame dialog.",
    }),
    invalidFrame: i18n._({
      id: "preview.invalidFrame",
      message: "Enter whole positive pixels for the size and a positive density.",
      comment: "Error of the custom frame dialog when the size or density is not a positive number.",
    }),
    frameTooLarge: (max: number) =>
      i18n._({
        id: "preview.frameTooLarge",
        message: "Width and height must be at most {max} logical pixels.",
        values: { max },
        comment: "Error of the custom frame dialog when the width or height is above the configured maximum.",
      }),
    densityOutOfRange: (max: number) =>
      i18n._({
        id: "preview.densityOutOfRange",
        message: "Pixel density must be between 1 and {max}.",
        values: { max },
        comment: "Error of the custom frame dialog when the pixel density is outside the configured range.",
      }),
    close: i18n._({
      id: "preview.close",
      message: "Close",
      comment: "Accessible name of the close button of the custom frame dialog.",
    }),
    theme: i18n._({ id: "preview.theme", message: "Theme", comment: "Label of the preview theme select." }),
    brand: i18n._({
      id: "preview.brand",
      message: "Brand",
      comment: "Label of the preview select of the theme's brand overlay (THM-003).",
    }),
    brandNone: i18n._({
      id: "preview.brandNone",
      message: "No overlay",
      comment: "Option of the brand select: the theme without a brand overlay.",
    }),
    themeLight: i18n._({ id: "preview.themeLight", message: "Light", comment: "Preview theme option." }),
    themeDark: i18n._({ id: "preview.themeDark", message: "Dark", comment: "Preview theme option." }),
    locale: i18n._({ id: "preview.locale", message: "Locale", comment: "Label of the preview locale select." }),
    textScale: i18n._({
      id: "preview.textScale",
      message: "Text scale",
      comment: "Label of the preview text scale select.",
    }),
    platform: i18n._({
      id: "preview.platform",
      message: "Platform style",
      comment: "Label of the select that picks Material or Cupertino for the preview.",
    }),
    platformFrame: i18n._({
      id: "preview.platformFrame",
      message: "Follow frame",
      comment: "Platform style option that follows the device frame.",
    }),
    material: i18n._({ id: "preview.material", message: "Material", comment: "Platform style option." }),
    cupertino: i18n._({ id: "preview.cupertino", message: "Cupertino", comment: "Platform style option." }),
    mock: i18n._({
      id: "preview.mock",
      message: "Data state",
      comment: "Label of the select that sets the mock state of the data sources.",
    }),
    mockOf: (source: string) =>
      i18n._({
        id: "preview.mockOf",
        message: "Data: {source}",
        values: { source },
        comment: "Label of one data source's mock state select; {source} is the source's name.",
      }),
    mockSuccess: i18n._({ id: "preview.mockSuccess", message: "Success", comment: "Mock data state option." }),
    mockLoading: i18n._({ id: "preview.mockLoading", message: "Loading", comment: "Mock data state option." }),
    mockEmpty: i18n._({ id: "preview.mockEmpty", message: "Empty", comment: "Mock data state option." }),
    mockError: i18n._({ id: "preview.mockError", message: "Error", comment: "Mock data state option." }),
    orientation: i18n._({
      id: "preview.orientation",
      message: "Orientation",
      comment: "Label of the select that turns the device frame between portrait and landscape.",
    }),
    portrait: i18n._({ id: "preview.portrait", message: "Portrait", comment: "Device orientation option." }),
    landscape: i18n._({ id: "preview.landscape", message: "Landscape", comment: "Device orientation option." }),
    chrome: i18n._({
      id: "preview.chrome",
      message: "Device chrome",
      comment:
        "Label of the select that shows or hides the device's status bar, camera cutout and home indicator on the screens.",
    }),
    chromeShown: i18n._({
      id: "preview.chromeShown",
      message: "Shown",
      comment: "Option: the status bar, camera cutout and home indicator are drawn on the screens.",
    }),
    chromeHidden: i18n._({
      id: "preview.chromeHidden",
      message: "Hidden",
      comment: "Option: the status bar, camera cutout and home indicator are not drawn (their space stays).",
    }),
    matrix: i18n._({
      id: "preview.matrix",
      message: "Device matrix",
      comment: "Label of the view that shows one screen in several device frames side by side.",
    }),
    matrixTitle: (screen: string) =>
      i18n._({
        id: "preview.matrixTitle",
        message: "{screen} on every device",
        values: { screen },
        comment: "Heading of the device matrix: the screen shown in every device frame.",
      }),
    matrixHelp: i18n._({
      id: "preview.matrixHelp",
      message:
        "One screen in every device frame at one scale, to see how it adapts to each size and size class. The preview switches apply to all of them.",
      comment: "Explains what the device matrix is for.",
    }),
    matrixClose: i18n._({
      id: "preview.matrixClose",
      message: "Back to the canvas",
      comment: "Button that closes the device matrix.",
    }),
    matrixSize: (width: number, height: number) =>
      i18n._({
        id: "preview.matrixSize",
        message: "{width} × {height}",
        values: { width, height },
        comment: "A device frame's size in logical pixels.",
      }),
    sizeClass: (sizeClass: string) =>
      i18n._({
        id: "preview.sizeClass",
        message: "{sizeClass, select, compact {Compact} medium {Medium} other {Expanded}}",
        values: { sizeClass },
        comment: "A window size class (WGT-010).",
      }),
    matrixCell: (name: string, width: number, height: number) =>
      i18n._({
        id: "preview.matrixCell",
        message: "{name}, {width} by {height}",
        values: { name, width, height },
        comment: "Accessible name of one device in the device matrix.",
      }),
    frameNames: {
      "android-small": i18n._({
        id: "preview.frame.android-small",
        message: "Small Android phone",
        comment: "Name of a device frame preset.",
      }),
      pixel: i18n._({ id: "preview.frame.pixel", message: "Pixel", comment: "Name of a device frame preset." }),
      "galaxy-a": i18n._({
        id: "preview.frame.galaxy-a",
        message: "Galaxy A-series",
        comment: "Name of a device frame preset.",
      }),
      "android-entry": i18n._({
        id: "preview.frame.android-entry",
        message: "Entry-level Android",
        comment: "Name of a device frame preset.",
      }),
      "iphone-se": i18n._({
        id: "preview.frame.iphone-se",
        message: "iPhone SE",
        comment: "Name of a device frame preset.",
      }),
      iphone: i18n._({ id: "preview.frame.iphone", message: "iPhone", comment: "Name of a device frame preset." }),
      "iphone-pro-max": i18n._({
        id: "preview.frame.iphone-pro-max",
        message: "iPhone Pro Max",
        comment: "Name of a device frame preset.",
      }),
      foldable: i18n._({
        id: "preview.frame.foldable",
        message: "Foldable",
        comment: "Name of a device frame preset.",
      }),
      ipad: i18n._({ id: "preview.frame.ipad", message: "iPad", comment: "Name of a device frame preset." }),
      "android-tablet": i18n._({
        id: "preview.frame.android-tablet",
        message: "Android tablet",
        comment: "Name of a device frame preset.",
      }),
      desktop: i18n._({
        id: "preview.frame.desktop",
        message: "Desktop window",
        comment: "Name of a device frame preset.",
      }),
    } as Record<string, string>,
  };
}
