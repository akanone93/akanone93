// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The words of the canvas–device comparison (PC-38). */
export function compareMessages(i18n: I18n) {
  return {
    title: i18n._({
      id: "compare.title",
      message: "The canvas and the device",
      comment: "Accessible name of the comparison of a device screenshot with the canvas's drawing of the same screen.",
    }),
    open: i18n._({
      id: "compare.open",
      message: "Compare with the canvas",
      comment: "Button that shows the device screenshot next to the canvas's drawing of the same screen.",
    }),
    mode: i18n._({ id: "compare.mode", message: "How to compare", comment: "Label of the comparison modes." }),
    side: i18n._({ id: "compare.side", message: "Side by side", comment: "Comparison mode." }),
    overlay: i18n._({
      id: "compare.overlay",
      message: "Overlay",
      comment: "Comparison mode: the canvas over the device.",
    }),
    difference: i18n._({
      id: "compare.difference",
      message: "Difference",
      comment: "Comparison mode: only what differs between the two lights up.",
    }),
    differenceHint: i18n._({
      id: "compare.differenceHint",
      message: "Black where the canvas and the device match; what lights up differs.",
      comment: "Explains the Difference comparison mode.",
    }),
    opacity: i18n._({
      id: "compare.opacity",
      message: "Canvas opacity",
      comment: "Slider of the canvas image over the device.",
    }),
    device: i18n._({ id: "compare.device", message: "Device", comment: "Caption of the device screenshot." }),
    canvas: i18n._({ id: "compare.canvas", message: "Canvas", comment: "Caption of the canvas's drawing." }),
    deviceAlt: i18n._({
      id: "compare.deviceAlt",
      message: "The screen as the paired device shows it",
      comment: "Alternative text of the device screenshot in the comparison.",
    }),
    canvasAlt: i18n._({
      id: "compare.canvasAlt",
      message: "The same screen as the canvas draws it",
      comment: "Alternative text of the canvas image in the comparison.",
    }),
    noCanvas: i18n._({
      id: "compare.noCanvas",
      message: "The canvas has not drawn this screen: open it on the canvas, then compare again.",
      comment: "Shown when the canvas has no image of the screen the device shows.",
    }),
  };
}
