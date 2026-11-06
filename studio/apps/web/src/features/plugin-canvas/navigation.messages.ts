// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The words of a link's parameters and transition and of Arrange by flow (PC-28, NAV-001, NAV-010). */
export function navigationMessages(i18n: I18n) {
  return {
    params: i18n._({
      id: "nav.params",
      message: "Parameters",
      comment: "Heading of what a link passes to the page it opens.",
    }),
    noParams: i18n._({
      id: "nav.noParams",
      message: "The page takes no parameters.",
      comment: "Shown when the opened page declares none.",
    }),
    paramHint: i18n._({
      id: "nav.paramHint",
      message: 'An expression (item.id), a number, true or false, or "text" in quotes',
      comment: "Placeholder of a link parameter's value field.",
    }),
    required: i18n._({ id: "nav.required", message: "required", comment: "Marks a parameter the page requires." }),
    missing: (names: string) =>
      i18n._({
        id: "nav.missing",
        message: "Missing: {names}. The page needs them to open.",
        values: { names },
        comment: "Shown when a link leaves out parameters its page requires.",
      }),
    opensWith: i18n._({
      id: "nav.opensWith",
      message: "Opens with",
      comment: "Label of the transition the opened page plays.",
    }),
    arrange: i18n._({
      id: "nav.arrange",
      message: "Arrange by flow",
      comment: "Command: lay the screens out in columns, a column per step from the entry screen.",
    }),
    arranged: i18n._({
      id: "nav.arranged",
      message: "Screens arranged by flow",
      comment: "Status after Arrange by flow.",
    }),
  };
}

export type NavigationMessages = ReturnType<typeof navigationMessages>;
