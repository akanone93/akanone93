// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the organisation home page (STU-012). */
export function homeMessages(i18n: I18n) {
  return {
    title: (name: string) =>
      i18n._({ id: "home.title", message: "Welcome to {name}", values: { name }, comment: "Heading of the organisation home; name is the organisation." }),
    caption: i18n._({ id: "home.caption", message: "Apps, plugins and approvals appear here as they become available.", comment: "Caption on the organisation home." }),
  };
}
