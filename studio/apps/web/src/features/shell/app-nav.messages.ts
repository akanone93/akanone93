// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the app-area navigation in the shell (STU-012, STU-100 to STU-116). */
export function appNavMessages(i18n: I18n) {
  return {
    label: i18n._({
      id: "appnav.label",
      message: "App areas",
      comment: "Accessible name of the navigation between the areas of one app.",
    }),
    releases: i18n._({
      id: "appnav.releases",
      message: "Releases",
      comment: "Entry of the app-area navigation; names one area of an app.",
    }),
    functions: i18n._({
      id: "appnav.functions",
      message: "Functions",
      comment: "Entry of the app-area navigation; names one area of an app.",
    }),
    data: i18n._({
      id: "appnav.data",
      message: "Data",
      comment: "Entry of the app-area navigation; names one area of an app.",
    }),
    localisation: i18n._({
      id: "appnav.localisation",
      message: "Localisation",
      comment: "Entry of the app-area navigation; names one area of an app.",
    }),
    design: i18n._({
      id: "appnav.design",
      message: "Design",
      comment: "Entry of the app-area navigation; names one area of an app.",
    }),
    analytics: i18n._({
      id: "appnav.analytics",
      message: "Analytics",
      comment: "Entry of the app-area navigation; names one area of an app.",
    }),
    tests: i18n._({
      id: "appnav.tests",
      message: "Tests",
      comment: "Entry of the app-area navigation; names one area of an app.",
    }),
    experiments: i18n._({
      id: "appnav.experiments",
      message: "Experiments",
      comment: "Entry of the app-area navigation; names one area of an app.",
    }),
    approvals: i18n._({
      id: "appnav.approvals",
      message: "Approvals",
      comment: "Entry of the app-area navigation; names one area of an app.",
    }),
    publish: i18n._({
      id: "appnav.publish",
      message: "Publish",
      comment: "Entry of the app-area navigation; names one area of an app.",
    }),
    devices: i18n._({
      id: "appnav.devices",
      message: "Devices",
      comment: "Entry of the app-area navigation; names one area of an app.",
    }),
    security: i18n._({
      id: "appnav.security",
      message: "Security",
      comment: "Entry of the app-area navigation; names one area of an app.",
    }),
    settings: i18n._({
      id: "appnav.settings",
      message: "Settings",
      comment: "Entry of the app-area navigation; names one area of an app.",
    }),
  };
}
