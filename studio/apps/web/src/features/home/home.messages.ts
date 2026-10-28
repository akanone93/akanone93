// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the organisation home page (STU-012). */
export function homeMessages(i18n: I18n) {
  return {
    title: (name: string) =>
      i18n._({
        id: "home.title",
        message: "Welcome to {name}",
        values: { name },
        comment: "Heading of the organisation home; name is the organisation.",
      }),
    caption: i18n._({
      id: "home.caption",
      message: "Apps, plugins and approvals appear here as they become available.",
      comment: "Caption on the organisation home.",
    }),
    appsTitle: i18n._({
      id: "home.apps.title",
      message: "Apps",
      comment: "Heading of the list of the organisation's apps.",
    }),
    appsEmpty: i18n._({
      id: "home.apps.empty",
      message: "This organisation has no apps yet.",
      comment: "Shown on the organisation home when it has no apps.",
    }),
    appsEmptyAsk: i18n._({
      id: "home.apps.empty-ask",
      message: "Ask an administrator to create one.",
      comment: "Second sentence of the empty apps state for a person who may not create apps.",
    }),
    appsLoadFailed: i18n._({
      id: "home.apps.load-failed",
      message: "The apps could not be loaded.",
      comment: "Shown when listing the organisation's apps failed.",
    }),
    appsMore: i18n._({
      id: "home.apps.more",
      message: "Show more apps",
      comment: "Button that loads the next page of the apps list.",
    }),
    openApp: (name: string) =>
      i18n._({
        id: "home.apps.open",
        message: "Open {name}",
        values: { name },
        comment: "Accessible name of an app's link in the apps list; name is the app.",
      }),
    createApp: i18n._({
      id: "home.apps.create",
      message: "Create app",
      comment: "Button, dialog title and submit label for creating an app; shown only with the app-manage permission.",
    }),
  };
}
