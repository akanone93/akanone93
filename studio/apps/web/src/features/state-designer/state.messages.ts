// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the state designer overlay (S5.5, STU-063, STA-002). */
export function stateMessages(i18n: I18n) {
  return {
    open: i18n._({
      id: "state.open",
      message: "State designer",
      comment: "Button in the right panel that opens the full-screen designer of the plugin's state entries.",
    }),
    title: i18n._({
      id: "state.title",
      message: "State designer",
      comment: "Title of the full-screen overlay in which a plugin's state entries are added, edited and removed.",
    }),
    close: i18n._({
      id: "state.close",
      message: "Close the state designer",
      comment: "Accessible name of the button that closes the state designer.",
    }),
    entries: i18n._({
      id: "state.entries",
      message: "State entries",
      comment: "Accessible name of the list of the plugin's state entries.",
    }),
    none: i18n._({
      id: "state.none",
      message: "This plugin has no state entries yet.",
      comment: "Shown when the plugin declares no state.",
    }),
    add: i18n._({
      id: "state.add",
      message: "Add state entry",
      comment: "Button that adds a new state entry to the plugin.",
    }),
    remove: i18n._({
      id: "state.remove",
      message: "Remove entry",
      comment: "Button that removes the chosen state entry unless something still reads it.",
    }),
    readOnly: i18n._({
      id: "state.readOnly",
      message: "Another person holds the draft lock, so the state is read-only.",
      comment: "Notice when the plugin's draft lock is held by someone else.",
    }),
    name: i18n._({
      id: "state.name",
      message: "Name",
      comment: "Label of the field with the name PXL uses to address a state entry.",
    }),
    container: i18n._({
      id: "state.container",
      message: "Shape",
      comment: "Label of the list that chooses whether a state entry is a single value, a list or a map.",
    }),
    single: i18n._({ id: "state.single", message: "Single value", comment: "Option: the entry holds one value." }),
    list: i18n._({ id: "state.list", message: "List", comment: "Option: the entry holds a list of values." }),
    map: i18n._({
      id: "state.map",
      message: "Map with text keys",
      comment: "Option: the entry holds a map from text keys to values.",
    }),
    base: i18n._({
      id: "state.base",
      message: "Type",
      comment: "Label of the list that chooses the type of the entry's value.",
    }),
    nullable: i18n._({
      id: "state.nullable",
      message: "May be empty",
      comment: "Label of the checkbox that makes the entry's type nullable.",
    }),
    rawType: i18n._({
      id: "state.rawType",
      message: "Type expression",
      comment: "Label of the field in which a type that the pickers cannot express is edited as text.",
    }),
    persistence: i18n._({
      id: "state.persistence",
      message: "Where it lives",
      comment: "Label of the list that chooses how long the entry's value is kept.",
    }),
    memory: i18n._({
      id: "state.persistence.memory",
      message: "In memory",
      comment: "Option: lost when the plugin closes.",
    }),
    session: i18n._({
      id: "state.persistence.session",
      message: "For the session",
      comment: "Option: kept until the app session ends.",
    }),
    persisted: i18n._({
      id: "state.persistence.persisted",
      message: "Saved on the device",
      comment: "Option: kept on the device between sessions.",
    }),
    secure: i18n._({
      id: "state.persistence.secure",
      message: "Saved in secure storage",
      comment: "Option: kept in the device's secure storage.",
    }),
    sensitive: i18n._({
      id: "state.sensitive",
      message: "Sensitive",
      comment: "Label of the checkbox that marks the entry as holding personal or secret data.",
    }),
    defaultValue: i18n._({
      id: "state.default",
      message: "Default (JSON)",
      comment: "Label of the field in which the entry's default value is typed as JSON.",
    }),
    computedNote: i18n._({
      id: "state.computed",
      message: "This entry is computed from an expression; edit the expression in the JSON editor.",
      comment: "Notice that a state entry has a computed expression, which this designer does not edit.",
    }),
    notJson: i18n._({
      id: "state.notJson",
      message: "The default is not valid JSON.",
      comment: "Error under the default field when the text is not JSON.",
    }),
    problems: i18n._({
      id: "state.problems",
      message: "Findings of the validator",
      comment: "Heading of the list of validator findings about the chosen state entry.",
    }),
    checking: i18n._({
      id: "state.checking",
      message: "Checking the entry…",
      comment: "Shown while the validator checks the chosen state entry.",
    }),
    valid: i18n._({
      id: "state.valid",
      message: "The validator found no problems with this entry.",
      comment: "Shown when the validator accepts the chosen state entry.",
    }),
    unavailable: i18n._({
      id: "state.unavailable",
      message: "The validator is not available, so the default is not checked.",
      comment: "Shown when the validator worker cannot check the entry.",
    }),
    uses: (count: number) =>
      i18n._({
        id: "state.uses",
        message: "{count, plural, =0 {Not used} one {Used # time} other {Used # times}}",
        values: { count },
        comment: "Beside a state entry: how many PXL expressions of the plugin read it.",
      }),
    problem: (kind: "empty" | "invalid" | "taken") =>
      kind === "empty"
        ? i18n._({
            id: "state.problem.empty",
            message: "Enter a name.",
            comment: "Error under the name field when it is empty.",
          })
        : kind === "taken"
          ? i18n._({
              id: "state.problem.taken",
              message: "Another entry has this name.",
              comment: "Error under the name field when the name is in use.",
            })
          : i18n._({
              id: "state.problem.invalid",
              message: "Start with a lower-case letter and use only letters and digits, such as loanAmount.",
              comment: "Error under the name field when the name is not lowerCamelCase.",
            }),
  };
}

export type StateMessages = ReturnType<typeof stateMessages>;
