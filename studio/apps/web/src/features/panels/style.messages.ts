// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The words of the style editors: colours and decorations (STU-062, THM-001). */
export function styleMessages(i18n: I18n) {
  return {
    none: i18n._({ id: "style.none", message: "None", comment: "A colour or decoration field with nothing set." }),
    themeColors: i18n._({
      id: "style.themeColors",
      message: "Theme colours",
      comment: "Heading of the colour roles of the app's Material colour scheme in the colour picker.",
    }),
    tokenColors: i18n._({
      id: "style.tokenColors",
      message: "Colour tokens",
      comment: "Heading of the app theme's other colour tokens in the colour picker.",
    }),
    custom: i18n._({
      id: "style.custom",
      message: "Custom",
      comment: "Heading of the custom colour in the colour picker.",
    }),
    hex: i18n._({ id: "style.hex", message: "Hex", comment: "Label of the hexadecimal colour field, e.g. 6750A4." }),
    opacity: i18n._({ id: "style.opacity", message: "Opacity", comment: "Label of the colour's opacity slider." }),
    clear: i18n._({ id: "style.clear", message: "Clear", comment: "Button that removes the colour or decoration." }),
    pick: (field: string) =>
      i18n._({
        id: "style.pick",
        message: "Choose {field}",
        values: { field },
        comment: "Accessible name of a colour field's button; {field} is the property, e.g. color.",
      }),
    fill: i18n._({ id: "style.fill", message: "Fill", comment: "Section of the decoration editor: the box's colour." }),
    gradient: i18n._({ id: "style.gradient", message: "Gradient", comment: "Section of the decoration editor." }),
    gradientKind: (kind: string) =>
      i18n._({
        id: "style.gradientKind",
        message: "{kind, select, none {No gradient} linear {Linear} radial {Radial} sweep {Sweep} other {Gradient}}",
        values: { kind },
        comment: "Choice of the kind of gradient of a decoration.",
      }),
    direction: i18n._({
      id: "style.direction",
      message: "Direction",
      comment: "Label of a linear gradient's direction.",
    }),
    directionName: (d: string) =>
      i18n._({
        id: "style.directionName",
        message:
          "{d, select, down {Top to bottom} across {Start to end} diagonal {Diagonal down} rising {Diagonal up} other {Direction}}",
        values: { d },
        comment: "A linear gradient's direction.",
      }),
    from: i18n._({ id: "style.from", message: "From", comment: "The first colour of a gradient." }),
    to: i18n._({ id: "style.to", message: "To", comment: "The last colour of a gradient." }),
    corners: i18n._({
      id: "style.corners",
      message: "Corners",
      comment: "Section of the decoration editor: the radii.",
    }),
    radius: i18n._({ id: "style.radius", message: "Radius", comment: "Label of the radius of every corner." }),
    corner: (c: string) =>
      i18n._({
        id: "style.corner",
        message:
          "{c, select, topLeft {Top start} topRight {Top end} bottomRight {Bottom end} bottomLeft {Bottom start} other {Corner}}",
        values: { c },
        comment: "Label of one corner's radius.",
      }),
    eachCorner: i18n._({
      id: "style.eachCorner",
      message: "Each corner",
      comment: "Switch that sets a radius per corner instead of one for all.",
    }),
    circle: i18n._({ id: "style.circle", message: "Circle", comment: "Switch that makes the decoration a circle." }),
    border: i18n._({ id: "style.border", message: "Border", comment: "Section of the decoration editor." }),
    width: i18n._({ id: "style.width", message: "Width", comment: "Label of a border's width." }),
    shadow: i18n._({ id: "style.shadow", message: "Shadow", comment: "Section of the decoration editor." }),
    elevation: (level: string) =>
      i18n._({
        id: "style.elevation",
        message: "{level, select, none {No shadow} custom {Custom shadow} other {Elevation {level}}}",
        values: { level },
        comment: "A shadow preset of the decoration editor, by Material elevation, e.g. Elevation 3.",
      }),
    textPreset: i18n._({
      id: "style.textPreset",
      message: "Style",
      comment: "Label of the text style select: a role of the type scale or a custom style.",
    }),
    typeRole: (role: string) =>
      i18n._({
        id: "style.typeRole",
        message:
          "{role, select, custom {Custom} displayLarge {Display large} displayMedium {Display medium} displaySmall {Display small} headlineLarge {Headline large} headlineMedium {Headline medium} headlineSmall {Headline small} titleLarge {Title large} titleMedium {Title medium} titleSmall {Title small} bodyLarge {Body large} bodyMedium {Body medium} bodySmall {Body small} labelLarge {Label large} labelMedium {Label medium} labelSmall {Label small} other {Style}}",
        values: { role },
        comment: "A role of Material 3's type scale, or a custom text style.",
      }),
    size: i18n._({ id: "style.size", message: "Size", comment: "Label of a text's font size." }),
    weight: i18n._({ id: "style.weight", message: "Weight", comment: "Label of a text's font weight (100 to 900)." }),
    italic: i18n._({ id: "style.italic", message: "Italic", comment: "Switch that makes a text italic." }),
    color: i18n._({ id: "style.color", message: "Colour", comment: "Label of a text's colour." }),
    tracking: i18n._({ id: "style.tracking", message: "Letter spacing", comment: "Label of a text's letter spacing." }),
    lineHeight: i18n._({
      id: "style.lineHeight",
      message: "Line height",
      comment: "Label of a text's line height, as a multiple of its size.",
    }),
    line: i18n._({ id: "style.line", message: "Line", comment: "Label of a text's decoration line select." }),
    decoration: (d: string) =>
      i18n._({
        id: "style.decoration",
        message:
          "{d, select, none {No line} underline {Underline} lineThrough {Strikethrough} overline {Overline} other {Line}}",
        values: { d },
        comment: "A text's decoration line.",
      }),
    iconSearch: i18n._({
      id: "style.iconSearch",
      message: "Search icons",
      comment: "Placeholder and label of the search field of the icon picker.",
    }),
    iconSet: (set: string) =>
      i18n._({
        id: "style.iconSet",
        message: "{set, select, material {Material} cupertino {Cupertino} other {Icons}}",
        values: { set },
        comment: "An icon set of the icon picker: Material Symbols or the Cupertino (iOS) icons.",
      }),
    iconLoading: i18n._({
      id: "style.iconLoading",
      message: "Loading icons…",
      comment: "Shown in the icon picker while the icon names load.",
    }),
    iconNoMatch: i18n._({
      id: "style.iconNoMatch",
      message: "No icons match.",
      comment: "Shown in the icon picker when no icon name contains the search.",
    }),
    iconMore: (count: number) =>
      i18n._({
        id: "style.iconMore",
        message: "{count, plural, one {# more icon: refine the search} other {# more icons: refine the search}}",
        values: { count },
        comment: "Below the icon grid when more icons match than it shows.",
      }),
  };
}

export type StyleMessages = ReturnType<typeof styleMessages>;
