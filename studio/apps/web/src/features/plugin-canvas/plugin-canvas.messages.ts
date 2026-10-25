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
    actionBar: i18n._({
      id: "pluginCanvas.actionBar",
      message: "Canvas actions",
      comment: "Accessible name of the toolbar under the plugin canvas.",
    }),
    undo: i18n._({
      id: "pluginCanvas.undo",
      message: "Undo",
      comment: "Button that undoes the last edit.",
    }),
    redo: i18n._({
      id: "pluginCanvas.redo",
      message: "Redo",
      comment: "Button that repeats the last undone edit.",
    }),
    zoomOut: i18n._({
      id: "pluginCanvas.zoomOut",
      message: "Zoom out",
      comment: "Button that zooms the canvas out.",
    }),
    zoomIn: i18n._({
      id: "pluginCanvas.zoomIn",
      message: "Zoom in",
      comment: "Button that zooms the canvas in.",
    }),
    zoomFit: i18n._({
      id: "pluginCanvas.zoomFit",
      message: "Fit",
      comment: "Button that fits all screens into the view.",
    }),
    currentScreen: i18n._({
      id: "pluginCanvas.currentScreen",
      message: "Current screen",
      comment: "Accessible name of the select that chooses the screen the screen actions apply to.",
    }),
    newScreen: i18n._({
      id: "pluginCanvas.newScreen",
      message: "New screen",
      comment: "Button and command that create a blank screen.",
    }),
    duplicateScreen: i18n._({
      id: "pluginCanvas.duplicateScreen",
      message: "Duplicate screen",
      comment: "Command that copies the current screen.",
    }),
    renameScreen: i18n._({
      id: "pluginCanvas.renameScreen",
      message: "Rename screen",
      comment: "Command that renames the current screen.",
    }),
    deleteScreen: i18n._({
      id: "pluginCanvas.deleteScreen",
      message: "Delete screen",
      comment: "Command that deletes the current screen.",
    }),
    screenTitle: i18n._({
      id: "pluginCanvas.screenTitle",
      message: "Screen title",
      comment: "Label of the text field that names a screen.",
    }),
    untitled: i18n._({
      id: "pluginCanvas.untitled",
      message: "New screen",
      comment: "Default title of a newly created screen.",
    }),
    copySuffix: i18n._({
      id: "pluginCanvas.copySuffix",
      message: "(copy)",
      comment: "Appended to the title of a duplicated screen.",
    }),
    create: i18n._({
      id: "pluginCanvas.create",
      message: "Create",
      comment: "Button that confirms creating a screen.",
    }),
    rename: i18n._({
      id: "pluginCanvas.rename",
      message: "Rename",
      comment: "Button that confirms renaming a screen.",
    }),
    cancel: i18n._({
      id: "pluginCanvas.cancel",
      message: "Cancel",
      comment: "Button that closes a dialog without changing anything.",
    }),
    close: i18n._({
      id: "pluginCanvas.close",
      message: "Close",
      comment: "Accessible name of the close button of a dialog.",
    }),
    delete: i18n._({
      id: "pluginCanvas.delete",
      message: "Delete",
      comment: "Button that confirms deleting a screen.",
    }),
    blockedEntry: i18n._({
      id: "pluginCanvas.blockedEntry",
      message: "This is the plugin's entry page. Choose another entry page before deleting it.",
      comment: "Reason a screen cannot be deleted.",
    }),
    blockedNavigate: (title: string) =>
      i18n._({
        id: "pluginCanvas.blockedNavigate",
        message: "The screen {title} navigates to it. Remove that step first.",
        values: { title },
        comment: "Reason a screen cannot be deleted: another screen navigates to it.",
      }),
    blockedTitle: i18n._({
      id: "pluginCanvas.blockedTitle",
      message: "This screen cannot be deleted",
      comment: "Heading of the dialog that lists what blocks deleting a screen.",
    }),
    confirmDelete: (title: string) =>
      i18n._({
        id: "pluginCanvas.confirmDelete",
        message: "Delete {title}? Its document is removed from the draft.",
        values: { title },
        comment: "Question that asks to confirm deleting a screen.",
      }),
    offline: i18n._({
      id: "pluginCanvas.offline",
      message: "You are offline. Nothing was changed; try again when the connection is back.",
      comment: "Shown when a screen action needs the server and the browser is offline.",
    }),
    pendingSave: i18n._({
      id: "pluginCanvas.pendingSave",
      message: "This screen has unsaved changes. Wait until they are saved, then try again.",
      comment: "Shown when a screen cannot be deleted while edits are waiting to be saved.",
    }),
    writeFailed: i18n._({
      id: "pluginCanvas.writeFailed",
      message: "The server did not accept the change.",
      comment: "Shown when a screen action failed.",
    }),
  };
}
