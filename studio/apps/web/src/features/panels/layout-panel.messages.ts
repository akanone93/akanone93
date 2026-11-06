// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The words of the layout editors (PC-22, WGT-010). */
export function layoutMessages(i18n: I18n) {
  return {
    autoLayout: i18n._({
      id: "layout.autoLayout",
      message: "Auto layout",
      comment: "Heading of the editor of how a row, column, flex or wrap lays out its children.",
    }),
    direction: i18n._({
      id: "layout.direction",
      message: "Direction",
      comment: "Whether children go in a row or a column.",
    }),
    row: i18n._({ id: "layout.row", message: "Row", comment: "Children side by side, along the reading direction." }),
    column: i18n._({ id: "layout.column", message: "Column", comment: "Children one under the other." }),
    alignPad: i18n._({
      id: "layout.alignPad",
      message: "Align children",
      comment: "Accessible name of the 3×3 grid that aligns a row's or column's children.",
    }),
    alignAt: (v: string, h: string) =>
      i18n._({
        id: "layout.alignAt",
        message:
          "Align {v, select, start {top} center {middle} other {bottom}} {h, select, start {start} center {centre} other {end}}",
        values: { v, h },
        comment: "One point of the alignment grid; start and end follow the reading direction.",
      }),
    distribution: i18n._({
      id: "layout.distribution",
      message: "Distribute",
      comment: "How a row's or column's free space is shared between its children.",
    }),
    distributionOf: (d: string) =>
      i18n._({
        id: "layout.distributionOf",
        message:
          "{d, select, packed {Packed} spaceBetween {Space between} spaceAround {Space around} spaceEvenly {Space evenly} other {Packed}}",
        values: { d },
        comment: "Flutter's MainAxisAlignment: packed by the alignment grid, or the free space between children.",
      }),
    gap: i18n._({ id: "layout.gap", message: "Gap", comment: "Space between the children of a row, column or wrap." }),
    runGap: i18n._({ id: "layout.runGap", message: "Run gap", comment: "Space between the lines (runs) of a wrap." }),
    size: i18n._({
      id: "layout.size",
      message: "Size",
      comment: "Whether a row or column is as long as its children (Hug) or as long as it may be (Fill).",
    }),
    stretch: i18n._({
      id: "layout.stretch",
      message: "Stretch children",
      comment: "Switch: every child takes the row's height or the column's width.",
    }),
    hug: i18n._({ id: "layout.hug", message: "Hug", comment: "Sized by its content." }),
    fill: i18n._({ id: "layout.fill", message: "Fill", comment: "Takes the free space of its row or column." }),
    fixed: i18n._({ id: "layout.fixed", message: "Fixed", comment: "Has a set size in pixels." }),
    width: i18n._({ id: "layout.width", message: "Width", comment: "A widget's width." }),
    height: i18n._({ id: "layout.height", message: "Height", comment: "A widget's height." }),
    flex: i18n._({
      id: "layout.flex",
      message: "Share",
      comment: "The flex factor: how much of the free space a filling child takes, relative to the others.",
    }),
    inRowOrColumn: (axis: string) =>
      i18n._({
        id: "layout.inRowOrColumn",
        message: "{axis, select, horizontal {In a row} other {In a column}}",
        comment: "Heading of how a widget is sized by the row or column it is in.",
        values: { axis },
      }),
    inStack: i18n._({
      id: "layout.inStack",
      message: "In a stack",
      comment: "Heading of the edges a widget is pinned to in the stack it is in.",
    }),
    pin: (edge: string) =>
      i18n._({
        id: "layout.pin",
        message:
          "{edge, select, left {Pin to the left} top {Pin to the top} right {Pin to the right} bottom {Pin to the bottom} width {Set the width} other {Set the height}}",
        values: { edge },
        comment: "A button of the stack pin editor; the widget keeps its distance to that edge.",
      }),
    pinValue: (edge: string) =>
      i18n._({
        id: "layout.pinValue",
        message: "{edge, select, left {Left} top {Top} right {Right} bottom {Bottom} width {Width} other {Height}}",
        values: { edge },
        comment: "Label of the distance to an edge, or of the size, of a pinned widget in a stack.",
      }),
    unpinned: i18n._({
      id: "layout.unpinned",
      message: "Not pinned: the stack's alignment places it.",
      comment: "Shown when a widget in a stack has no pins.",
    }),
    overrides: i18n._({
      id: "layout.overrides",
      message: "Size-class overrides",
      comment: "Heading of the properties a widget sets differently on medium or expanded windows.",
    }),
    overrideCount: (bp: string, n: number) =>
      i18n._({
        id: "layout.overrideCount",
        message: "{bp, select, medium {Medium} other {Expanded}}: {n}",
        values: { bp, n },
        comment: "A size class and how many properties it overrides, e.g. Medium: 2.",
      }),
    editOverrides: (bp: string) =>
      i18n._({
        id: "layout.editOverrides",
        message: "Edit the {bp, select, medium {Medium} other {Expanded}} overrides",
        values: { bp },
        comment: "Button that switches the inspector to a size class.",
      }),
    clearOverrides: (bp: string) =>
      i18n._({
        id: "layout.clearOverrides",
        message: "Clear the {bp, select, medium {Medium} other {Expanded}} overrides",
        values: { bp },
        comment: "Button that removes every property a size class overrides.",
      }),
    editingClass: (bp: string) =>
      i18n._({
        id: "layout.editingClass",
        message:
          "Editing {bp, select, medium {Medium (600–839 dp)} other {Expanded (840 dp and wider)}}: a value set here overrides the smaller classes.",
        values: { bp },
        comment: "Shown while the inspector edits a size class's overrides (WGT-010).",
      }),
    least: i18n._({ id: "layout.least", message: "Min", comment: "Column heading: the least width or height." }),
    most: i18n._({
      id: "layout.most",
      message: "Max",
      comment: "Column heading: the most width or height; empty is unbounded.",
    }),
    minOf: (what: string) =>
      i18n._({
        id: "layout.minOf",
        message: "Least {what}",
        values: { what },
        comment: "Accessible name of a constraint field; {what} is Width or Height.",
      }),
    maxOf: (what: string) =>
      i18n._({
        id: "layout.maxOf",
        message: "Most {what}",
        values: { what },
        comment: "Accessible name of a constraint field; {what} is Width or Height.",
      }),
    resetLayout: i18n._({
      id: "layout.resetLayout",
      message: "Reset layout",
      comment: "Button that clears the widget's layout properties at the size class being edited.",
    }),
  };
}

export type LayoutMessages = ReturnType<typeof layoutMessages>;
