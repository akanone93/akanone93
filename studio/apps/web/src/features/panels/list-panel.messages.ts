// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The words of the list panel and of the views a list shows by default (PC-24, WGT-012, DAT-011). */
export function listMessages(i18n: I18n) {
  return {
    list: i18n._({
      id: "list.title",
      message: "List from data",
      comment: "Heading of the editor of a list, grid or page view's items and states.",
    }),
    items: i18n._({
      id: "list.items",
      message: "Items",
      comment: "The list value a list's item template is repeated for.",
    }),
    itemsFrom: (expr: string, shown: string) =>
      i18n._({
        id: "list.itemsFrom",
        message: "{expr} · {shown}",
        values: { expr, shown },
        comment:
          "A list's items: the expression they are bound to and what the canvas draws, e.g. data.orders.value · [12].",
      }),
    itemsLiteral: (n: number) =>
      i18n._({
        id: "list.itemsLiteral",
        message: "{n, plural, one {# item written in the page} other {# items written in the page}}",
        values: { n },
        comment: "A list's items are a literal list in the page.",
      }),
    itemsNone: i18n._({
      id: "list.itemsNone",
      message: "Bind Items to a list or a data source to repeat the item template.",
      comment: "Shown when a list's items are not set.",
    }),
    show: i18n._({ id: "list.show", message: "Show", comment: "Which state of the list the canvas draws." }),
    stateName: (state: string) =>
      i18n._({
        id: "list.stateName",
        message:
          "{state, select, auto {From data} items {Items} empty {Empty} loading {Loading} error {Error} more {Loading more} other {State}}",
        values: { state },
        comment:
          "A state a list is drawn in; From data follows the Preview's data state; Loading more is the end of a paged list.",
      }),
    states: i18n._({
      id: "list.states",
      message: "States",
      comment: "Heading of a list's empty, loading and error views.",
    }),
    designed: i18n._({
      id: "list.designed",
      message: "Designed",
      comment: "A state of the list has a view of its own.",
    }),
    add: (state: string) =>
      i18n._({
        id: "list.add",
        message: "{state, select, empty {Add an empty view} loading {Add a loading view} other {Add an error view}}",
        values: { state },
        comment: "Button that gives a list a view for one of its states, starting from a template.",
      }),
    defaultEmpty: i18n._({
      id: "list.defaultEmpty",
      message: "Nothing here yet",
      comment: "The text of a list's empty view.",
    }),
    defaultError: i18n._({
      id: "list.defaultError",
      message: "Couldn't load",
      comment: "The text of a list's error view.",
    }),
    retry: i18n._({ id: "list.retry", message: "Try again", comment: "The button of a list's error view." }),
  };
}

export type ListMessages = ReturnType<typeof listMessages>;
