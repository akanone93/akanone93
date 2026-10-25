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
    drawer: i18n._({
      id: "problems.drawer",
      message: "Drawer",
      comment: "Accessible name of the drawer below the canvas that holds tabs such as Problems.",
    }),
    none: i18n._({
      id: "problems.none",
      message: "No problems found.",
      comment: "Shown in the Problems tab when the validator found nothing.",
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
