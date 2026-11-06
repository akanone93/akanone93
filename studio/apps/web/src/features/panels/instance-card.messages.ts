// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The words of the selected instance's card and canvas marks (PC-37). */
export function instanceCardMessages(i18n: I18n) {
  return {
    title: (name: string) =>
      i18n._({
        id: "instanceCard.title",
        message: "Instance of {name}",
        values: { name },
        comment: "Heading: the selected widget is an instance of this component.",
      }),
    version: (v: number) =>
      i18n._({
        id: "instanceCard.version",
        message: "Version {v}",
        values: { v },
        comment: "The component version the instance uses.",
      }),
    overrides: (n: number) =>
      i18n._({
        id: "instanceCard.overrides",
        message: "{n, plural, =0 {no overrides} one {# override} other {# overrides}}",
        values: { n },
        comment: "How many props and slots the instance sets itself.",
      }),
    newer: (v: number) =>
      i18n._({
        id: "instanceCard.newer",
        message: "Version {v} is available: the app adopts it from the component's page",
        values: { v },
        comment: "The component has a newer version than the one this instance uses (STU-085).",
      }),
    missing: i18n._({
      id: "instanceCard.missing",
      message: "The app does not have this component: the canvas cannot draw it",
      comment: "The instance names a component the app's documents do not hold.",
    }),
    isolate: i18n._({
      id: "instanceCard.isolate",
      message: "Isolate",
      comment: "Button that dims everything on the canvas but this instance.",
    }),
    leave: i18n._({ id: "instanceCard.leave", message: "Show everything", comment: "Button that ends the isolation." }),
    open: i18n._({
      id: "instanceCard.open",
      message: "Open component",
      comment: "Button that opens the component's page: its versions, instances and adoption.",
    }),
    tag: (name: string, v: number, n: number) =>
      i18n._({
        id: "instanceCard.tag",
        message: "{name} · v{v} · {n, plural, =0 {no overrides} one {# override} other {# overrides}}",
        values: { name, v, n },
        comment: "Canvas tag under a selected instance.",
      }),
  };
}
