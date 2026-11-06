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
    imageSource: (kind: string) =>
      i18n._({
        id: "style.imageSource",
        message: "{kind, select, asset {App images} url {Web address} other {Image}}",
        values: { kind },
        comment: "Where an image comes from, in the image picker: the app's uploaded images or a web address.",
      }),
    imageEmpty: i18n._({
      id: "style.imageEmpty",
      message: "No images yet. Upload them in the Assets tab.",
      comment: "Shown in the image picker when the app has no uploaded images.",
    }),
    imageUrlNote: i18n._({
      id: "style.imageUrlNote",
      message: "The device loads a web image; the canvas shows its placeholder.",
      comment: "Below the web address field of the image picker.",
    }),
    imageWeb: i18n._({ id: "style.imageWeb", message: "Web image", comment: "An image source that is a web address." }),
    statesOf: (field: string) =>
      i18n._({
        id: "style.statesOf",
        message: "States of {field}",
        values: { field },
        comment:
          "Accessible name of the row of state chips (Default, Pressed, Hovered…) of a property; {field} is the property.",
      }),
    stateName: (state: string) =>
      i18n._({
        id: "style.stateName",
        message:
          "{state, select, value {Default} pressed {Pressed} hovered {Hovered} focused {Focused} disabled {Disabled} selected {Selected} error {Error} dragged {Dragged} other {State}}",
        values: { state },
        comment:
          "An interaction state of a widget, as Flutter's WidgetState names it; Default applies when no other does.",
      }),
    stateUnset: (state: string) =>
      i18n._({
        id: "style.stateUnset",
        message: "{state}: not set, uses Default",
        values: { state },
        comment: "Tooltip of a state chip whose state has no value of its own; {state} is the state's name.",
      }),
    stateShown: i18n._({
      id: "style.stateShown",
      message: "The canvas shows the widget in this state.",
      comment: "Below a state's editor: the selected widget is drawn in the state being edited.",
    }),
    stateFallsBack: i18n._({
      id: "style.stateFallsBack",
      message: "Not set: this state uses Default. The canvas shows it.",
      comment: "Below the editor of a state with no value of its own; the widget is drawn in that state.",
    }),
    shapeOf: (field: string) =>
      i18n._({
        id: "style.shapeOf",
        message: "Shape of {field}",
        values: { field },
        comment: "Accessible name of the shape kind menu; {field} is the property, e.g. shape.",
      }),
    shapeKind: (kind: string) =>
      i18n._({
        id: "style.shapeKind",
        message:
          "{kind, select, roundedRectangle {Rounded rectangle} stadium {Pill} circle {Circle} beveledRectangle {Bevelled rectangle} continuousRectangle {Smooth rectangle} other {Shape}}",
        values: { kind },
        comment: "A kind of shape of a button or card, as Flutter's ShapeBorder kinds; Pill is a stadium.",
      }),
    eachCornerSet: i18n._({
      id: "style.eachCornerSet",
      message: "Set per corner",
      comment: "Shown instead of the radius field when the corners have different radii.",
    }),
    eccentricity: i18n._({
      id: "style.eccentricity",
      message: "Eccentricity",
      comment: "How elliptical a circle shape may become, from 0 (a circle) to 1.",
    }),
    outline: i18n._({
      id: "style.outline",
      message: "Outline",
      comment: "A shape's border line: its colour and width.",
    }),
    outlineWidth: i18n._({
      id: "style.outlineWidth",
      message: "Outline width",
      comment: "Accessible name of the width field of a shape's outline.",
    }),
    bindToken: (field: string) =>
      i18n._({
        id: "style.bindToken",
        message: "Use a theme token for {field}",
        values: { field },
        comment: "Button beside a number that binds it to a dimension token of the theme, e.g. space.md.",
      }),
    unbindToken: (token: string) =>
      i18n._({
        id: "style.unbindToken",
        message: "Unlink {token}: keep its value",
        values: { token },
        comment: "Button that replaces a bound theme token by its value in pixels; {token} is its path, e.g. space.md.",
      }),
    copyStyle: i18n._({
      id: "style.copyStyle",
      message: "Copy style",
      comment: "Command: copy the selected widget's colours, decoration, shape and text style.",
    }),
    pasteStyle: i18n._({
      id: "style.pasteStyle",
      message: "Paste style",
      comment: "Command: apply the copied style to the selected widgets.",
    }),
    styleCopied: (type: string) =>
      i18n._({
        id: "style.styleCopied",
        message: "Style of {type} copied",
        values: { type },
        comment: "Status after copying a widget's style; {type} is the widget type, e.g. FilledButton.",
      }),
    stylePasted: (n: number) =>
      i18n._({
        id: "style.stylePasted",
        message: "{n, plural, one {Style pasted on # widget} other {Style pasted on # widgets}}",
        values: { n },
        comment: "Status after pasting a copied style.",
      }),
    styleNotTaken: i18n._({
      id: "style.styleNotTaken",
      message: "None of the copied style fits the selection",
      comment: "Status when the selected widgets declare none of the copied style's properties.",
    }),
  };
}

export type StyleMessages = ReturnType<typeof styleMessages>;
