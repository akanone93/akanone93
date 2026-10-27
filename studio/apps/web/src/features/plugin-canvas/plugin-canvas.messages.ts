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
    wrapCommand: i18n._({
      id: "pluginCanvas.command.wrap",
      message: "Wrap selection in a column",
      comment: "Command that wraps the selected canvas elements in a new column.",
    }),
    unwrapCommand: i18n._({
      id: "pluginCanvas.command.unwrap",
      message: "Unwrap selection",
      comment: "Command that replaces the selected container on the canvas by its children.",
    }),
    duplicateCommand: i18n._({
      id: "pluginCanvas.command.duplicate",
      message: "Duplicate selection",
      comment: "Command that copies the selected canvas elements right after themselves.",
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
    publish: i18n._({
      id: "pluginCanvas.publish",
      message: "Publish",
      comment: "Primary button of the canvas action bar that opens the release flow.",
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
    ghostLinks: (count: number) =>
      i18n._({
        id: "pluginCanvas.ghostLinks",
        message: "{count, plural, one {# link} other {# links}}",
        values: { count },
        comment:
          "Number of navigate actions of this plugin that lead to a ghost screen or native route, drawn on its card.",
      }),
    ghostLeaveTitle: i18n._({
      id: "pluginCanvas.ghostLeaveTitle",
      message: "Open another plugin?",
      comment: "Title of the confirmation that opens the canvas of the plugin a ghost screen belongs to.",
    }),
    ghostLeaveBody: (screen: string) =>
      i18n._({
        id: "pluginCanvas.ghostLeaveBody",
        message:
          "{screen} belongs to another plugin. Your changes here are saved. Leave this canvas and open that screen?",
        values: { screen },
        comment: "Body of the confirmation that opens the canvas of the plugin a ghost screen belongs to.",
      }),
    ghostOpen: i18n._({
      id: "pluginCanvas.ghostOpen",
      message: "Open screen",
      comment: "Button that leaves this plugin's canvas and opens the ghost screen on its own plugin's canvas.",
    }),
    ghostStay: i18n._({
      id: "pluginCanvas.ghostStay",
      message: "Stay here",
      comment: "Button that closes the confirmation and keeps the current canvas.",
    }),
    ghostPulse: (screen: string) =>
      i18n._({
        id: "pluginCanvas.ghostPulse",
        message: "Showing {screen}",
        values: { screen },
        comment: "Announced to screen readers when the canvas pans to a screen and highlights it.",
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
    screenMenu: i18n._({
      id: "pluginCanvas.screenMenu",
      message: "Screen menu",
      comment: "Accessible name of the button that opens the menu of actions on the current screen.",
    }),
    moveScreen: i18n._({
      id: "pluginCanvas.moveScreen",
      message: "Move to plugin",
      comment: "Menu entry and command that move the current screen, with its links, to another plugin (D23).",
    }),
    move: i18n._({
      id: "pluginCanvas.move",
      message: "Move",
      comment: "Button that confirms moving a screen to the chosen plugin.",
    }),
    moveTarget: i18n._({
      id: "pluginCanvas.moveTarget",
      message: "Target plugin",
      comment: "Label of the choice of the plugin a screen moves to.",
    }),
    moveRefused: (reason: string) =>
      i18n._({
        id: "pluginCanvas.moveRefused",
        message: "The screen cannot move: {reason}",
        values: { reason },
        comment: "Shown when a move to another plugin is refused; reason is one of the moveReason messages.",
      }),
    moveReasonEntry: i18n._({
      id: "pluginCanvas.moveReasonEntry",
      message: "it is the entry screen; choose another entry first.",
      comment: "Reason a screen cannot move: it is its plugin's entry (STU-071).",
    }),
    moveReasonKey: i18n._({
      id: "pluginCanvas.moveReasonKey",
      message: "the target plugin already has a screen with this key.",
      comment: "Reason a screen cannot move: the key is taken in the target plugin.",
    }),
    moveReasonPrivate: i18n._({
      id: "pluginCanvas.moveReasonPrivate",
      message: "it uses a component that is private to this plugin.",
      comment: "Reason a screen cannot move: it uses a component that is not exported.",
    }),
    moveReason: (reason: "entry" | "key-taken" | "private-component" | "unknown" | "same-plugin") => {
      switch (reason) {
        case "entry":
          return i18n._({
            id: "pluginCanvas.moveReasonEntry",
            message: "it is the entry screen; choose another entry first.",
            comment: "Reason a screen cannot move: it is its plugin's entry (STU-071).",
          });
        case "key-taken":
          return i18n._({
            id: "pluginCanvas.moveReasonKey",
            message: "the target plugin already has a screen with this key.",
            comment: "Reason a screen cannot move: the key is taken in the target plugin.",
          });
        case "private-component":
          return i18n._({
            id: "pluginCanvas.moveReasonPrivate",
            message: "it uses a component that is private to this plugin.",
            comment: "Reason a screen cannot move: it uses a component that is not exported.",
          });
        default:
          return i18n._({
            id: "pluginCanvas.moveReasonOther",
            message: "the screen or the target is unknown.",
            comment: "Reason a screen cannot move: the screen or the target plugin could not be found.",
          });
      }
    },
    changeRefused: i18n._({
      id: "pluginCanvas.changeRefused",
      message: "The change was not applied because a document changed meanwhile. Nothing was written.",
      comment: "Shown when an atomic change across documents is refused by the server; nothing was written.",
    }),
    redirectTitle: i18n._({
      id: "pluginCanvas.redirectTitle",
      message: "Redirect links",
      comment: "Title of the dialog that chooses the screen links are redirected to.",
    }),
    redirectTarget: i18n._({
      id: "pluginCanvas.redirectTarget",
      message: "New target screen",
      comment: "Label of the choice of the screen links are redirected to.",
    }),
    redirect: i18n._({
      id: "pluginCanvas.redirect",
      message: "Redirect",
      comment: "Button that confirms redirecting links to the chosen screen.",
    }),
    deleteConfirmAction: i18n._({
      id: "pluginCanvas.deleteConfirmAction",
      message: "Delete screen",
      comment:
        "Label of the destructive button of the type-to-confirm dialog that deletes a screen of an app with a production release.",
    }),
    deleteProductionNote: (name: string) =>
      i18n._({
        id: "pluginCanvas.deleteProductionNote",
        message: "{name} is part of an app with a production release. Deleting it changes the next release.",
        values: { name },
        comment: "Explains why deleting a screen needs typing its name (STU-074).",
      }),
    laneTitle: (title: string) =>
      i18n._({
        id: "pluginCanvas.laneTitle",
        message: "Link: {title}",
        values: { title },
        comment: "Title of the dialog of one navigation link picked on the canvas; title names source and target.",
      }),
    laneRedirect: i18n._({
      id: "pluginCanvas.laneRedirect",
      message: "Redirect to another screen",
      comment: "Button that points the picked link at another screen (STU-046).",
    }),
    laneRemove: i18n._({
      id: "pluginCanvas.laneRemove",
      message: "Remove link",
      comment: "Button that removes the picked navigation link (STU-046).",
    }),
    laneReadOnly: i18n._({
      id: "pluginCanvas.laneReadOnly",
      message: "Take the editing lock to change this link.",
      comment: "Shown in the dialog of a picked link when the plugin cannot be edited.",
    }),
    laneUnavailable: i18n._({
      id: "pluginCanvas.laneUnavailable",
      message: "This link is not set directly on a widget event, so it cannot be changed here.",
      comment: "Shown when a picked link cannot be redirected or removed from the canvas.",
    }),
  };
}
