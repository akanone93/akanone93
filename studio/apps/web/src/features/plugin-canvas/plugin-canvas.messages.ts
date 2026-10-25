// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the plugin canvas (STU-050, STU-056). */
export function pluginCanvasMessages(i18n: I18n) {
  return {
    title: i18n._({
      id: "pluginCanvas.title",
      message: "Plugin canvas",
      comment: "Accessible name of the canvas that shows all screens of a plugin.",
    }),
    screens: i18n._({
      id: "pluginCanvas.screens",
      message: "Screens in navigation order",
      comment: "Label of the list of the plugin's screens, in the order the navigation flow reaches them.",
    }),
    overflow: (name: string) =>
      i18n._({
        id: "pluginCanvas.overflow",
        message: "{name}: content beyond the screen",
        values: { name },
        comment: "Describes the translucent region below a screen that holds content taller than the device screen.",
      }),
    selectionNone: i18n._({
      id: "pluginCanvas.selectionNone",
      message: "Nothing selected",
      comment: "Announced to screen readers when the canvas selection becomes empty.",
    }),
    selectionOne: (name: string) =>
      i18n._({
        id: "pluginCanvas.selectionOne",
        message: "{name} selected",
        values: { name },
        comment: "Announced to screen readers when one element is selected on the canvas.",
      }),
    selectionMany: (count: number) =>
      i18n._({
        id: "pluginCanvas.selectionMany",
        message: "{count} elements selected",
        values: { count },
        comment: "Announced to screen readers when several elements are selected on the canvas.",
      }),
    zoom: (percent: number) =>
      i18n._({
        id: "pluginCanvas.zoom",
        message: "Zoom {percent}%",
        values: { percent },
        comment: "Current zoom of the canvas in percent.",
      }),
    empty: i18n._({
      id: "pluginCanvas.empty",
      message: "This plugin has no screens yet.",
      comment: "Shown on the plugin canvas when the plugin has no screens.",
    }),
    loading: i18n._({
      id: "pluginCanvas.loading",
      message: "Loading screens",
      comment: "Shown while the plugin's screens load.",
    }),
    failed: i18n._({
      id: "pluginCanvas.failed",
      message: "The screens could not be loaded.",
      comment: "Shown when loading the plugin's screens failed.",
    }),
  };
}
