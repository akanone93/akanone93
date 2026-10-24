// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/**
 * Messages of the application shell (studio-development §7.4): key, English source, translator note.
 * They resolve against the instance the app owns, so there is no global catalogue.
 */
export function shellMessages(i18n: I18n) {
  return {
    appTitle: i18n._({
      id: "shell.app.title",
      message: "Plux Studio",
      comment: "Name of the application, shown as the browser tab title.",
    }),
  };
}
