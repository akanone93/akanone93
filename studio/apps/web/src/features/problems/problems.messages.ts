// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the Problems tab and the problem counts (S2.1, F9). */
export function problemsMessages(i18n: I18n) {
  return {
    tab: i18n._({
      id: "problems.tab",
      message: "Problems",
      comment: "Label of the drawer tab that lists the validator's findings.",
    }),
    none: i18n._({
      id: "problems.none",
      message: "No problems found.",
      comment: "Shown in the Problems tab when the validator found nothing.",
    }),
    whyAssetMissing: i18n._({
      id: "problems.why.assetMissing",
      message:
        "The app's asset list names this file, but no file of that name is stored: it was deleted or its upload did not finish. Widgets that show it have no image. Upload a file under the same name to bring them back.",
      comment: "Explains a missing asset file in the Problems tab, with what to do.",
    }),
    whyPluginNotListed: i18n._({
      id: "problems.why.pluginNotListed",
      message: "This plugin exists but app.json does not list it, so the app does not include it. Add it to the list.",
      comment: "Explains a plugin missing from app.json's plugins list.",
    }),
    whyPluginNoDirectory: i18n._({
      id: "problems.why.pluginNoDirectory",
      message: "app.json lists a plugin that no longer exists (it was deleted or renamed). Remove it from the list.",
      comment: "Explains a plugin key in app.json that has no plugin.",
    }),
    whyDuplicateRoute: i18n._({
      id: "problems.why.duplicateRoute",
      message:
        "Two screens use the same route name; route names are unique in the app. Rename one in its Page settings (Route).",
      comment: "Explains a duplicate route name, with where to change it.",
    }),
    whyUnresolved: i18n._({
      id: "problems.why.unresolved",
      message:
        "Something here points at an item that does not exist (deleted or renamed). Point it at an existing one.",
      comment: "Explains an unresolved reference.",
    }),
    fixUploadAsset: (file: string) =>
      i18n._({
        id: "problems.fix.uploadAsset",
        message: "Upload {file}",
        values: { file },
        comment: "Button that uploads a file under the name a missing asset had.",
      }),
    fixListPlugin: (plugin: string) =>
      i18n._({
        id: "problems.fix.listPlugin",
        message: "Add {plugin} to app.json",
        values: { plugin },
        comment: "Button that adds a plugin key to app.json's plugins list.",
      }),
    fixUnlistPlugin: (plugin: string) =>
      i18n._({
        id: "problems.fix.unlistPlugin",
        message: "Remove {plugin} from app.json",
        values: { plugin },
        comment: "Button that removes a key without a plugin from app.json's plugins list.",
      }),
    fixOpenJson: i18n._({
      id: "problems.fix.openJson",
      message: "Open in JSON editor",
      comment: "Button that opens the finding's file in the JSON editor.",
    }),
    fixFailed: i18n._({
      id: "problems.fix.failed",
      message: "The fix was not applied.",
      comment: "Shown under a finding when its fix failed; the server's reason follows.",
    }),
    validating: i18n._({
      id: "problems.validating",
      message: "Checking the plugin…",
      comment: "Shown in the Problems tab while the validator runs on the plugin that was just opened.",
    }),
    unavailable: i18n._({
      id: "problems.unavailable",
      message: "The validator could not run.",
      comment: "Shown in the Problems tab when the validator failed to start or crashed.",
    }),
    project: i18n._({
      id: "problems.project",
      message: "Project",
      comment: "Heading of the problems that belong to the whole project rather than to one file.",
    }),
    counts: i18n._({
      id: "problems.counts",
      message: "Problem counts",
      comment: "Accessible name of the group in the action bar that shows how many errors and warnings there are.",
    }),
    errors: (count: number) =>
      i18n._({
        id: "problems.errors",
        message: "{count, plural, one {# error} other {# errors}}",
        values: { count },
        comment: "Accessible name of the error count in the action bar.",
      }),
    warnings: (count: number) =>
      i18n._({
        id: "problems.warnings",
        message: "{count, plural, one {# warning} other {# warnings}}",
        values: { count },
        comment: "Accessible name of the warning count in the action bar.",
      }),
    severityError: i18n._({
      id: "problems.severity.error",
      message: "Error",
      comment: "Accessible name of the icon of an error in the Problems tab.",
    }),
    severityWarning: i18n._({
      id: "problems.severity.warning",
      message: "Warning",
      comment: "Accessible name of the icon of a warning in the Problems tab.",
    }),
    severityInfo: i18n._({
      id: "problems.severity.info",
      message: "Information",
      comment: "Accessible name of the icon of an information item in the Problems tab.",
    }),
  };
}
