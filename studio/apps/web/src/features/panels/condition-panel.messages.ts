// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The words of the conditions panel (PC-25, SCH-023). */
export function conditionMessages(i18n: I18n) {
  return {
    title: i18n._({
      id: "condition.title",
      message: "Conditions",
      comment: "Heading of the editor of when a widget shows and which branch it draws.",
    }),
    visibleWhen: i18n._({
      id: "condition.visibleWhen",
      message: "Visible when",
      comment: "Label of a widget's visibility condition, a PXL expression such as page.count > 0.",
    }),
    visiblePlaceholder: i18n._({
      id: "condition.visiblePlaceholder",
      message: "Always (e.g. page.items.isNotEmpty)",
      comment: "Placeholder of the visibility condition field when it is empty.",
    }),
    condition: i18n._({ id: "condition.condition", message: "Condition", comment: "An If widget's condition." }),
    value: i18n._({
      id: "condition.value",
      message: "Value",
      comment: "A Match widget's value, compared with its cases.",
    }),
    draw: i18n._({ id: "condition.draw", message: "Draw", comment: "Which branch or visibility the canvas draws." }),
    worked: (result: string) =>
      i18n._({
        id: "condition.worked",
        message:
          "{result, select, true {Now true: the device shows it} false {Now false: the device hides it} other {Only the device can work this out}}",
        values: { result },
        comment: "What a condition is at design time, from page mocks and state defaults.",
      }),
    pick: (pick: string) =>
      i18n._({
        id: "condition.pick",
        message:
          "{pick, select, auto {From data} then {Then} else {Else} otherwise {Otherwise} shown {Shown} hidden {Hidden} other {Branch}}",
        values: { pick },
        comment: "A branch the canvas draws; From data follows the condition's design-time value.",
      }),
  };
}

export type ConditionMessages = ReturnType<typeof conditionMessages>;
