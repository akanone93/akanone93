// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";
import type { ValidatorTexts } from "./forms-model.ts";

/** The words of the form editors and the validators' design-time messages (PC-26, STA-020). */
export function formsMessages(i18n: I18n) {
  return {
    forms: i18n._({ id: "forms.forms", message: "Forms", comment: "Section of the page settings: the page's forms." }),
    addForm: i18n._({ id: "forms.addForm", message: "Add a form", comment: "Button that adds a form to the page." }),
    removeForm: (name: string) =>
      i18n._({
        id: "forms.removeForm",
        message: "Remove the form {name}",
        values: { name },
        comment: "Button that removes a form.",
      }),
    formName: i18n._({
      id: "forms.formName",
      message: "Form name",
      comment: "Label of a form's name; bindings read it as page.<name>.",
    }),
    field: i18n._({ id: "forms.field", message: "Field", comment: "A field of a form." }),
    fieldName: i18n._({ id: "forms.fieldName", message: "Field name", comment: "Label of a form field's name." }),
    fieldType: i18n._({ id: "forms.fieldType", message: "Type", comment: "Label of a form field's type." }),
    initial: i18n._({
      id: "forms.initial",
      message: "Initial value",
      comment: "Label of a form field's initial value.",
    }),
    addField: i18n._({ id: "forms.addField", message: "Add a field", comment: "Button that adds a field to a form." }),
    removeField: (name: string) =>
      i18n._({
        id: "forms.removeField",
        message: "Remove the field {name}",
        values: { name },
        comment: "Button that removes a form field.",
      }),
    addValidator: i18n._({
      id: "forms.addValidator",
      message: "Add a check",
      comment: "Menu that adds a validator to a field.",
    }),
    validatorName: (kind: string) =>
      i18n._({
        id: "forms.validatorName",
        message:
          "{kind, select, required {Required} length {Length} range {Range} regex {Pattern} email {Email} phone {Phone} iban {IBAN} dateRange {Date range} decimalPrecision {Decimals} custom {Custom rule} async {Server check} other {Check}}",
        values: { kind },
        comment: "A built-in validator of a form field (STA-020).",
      }),
    option: (key: string) =>
      i18n._({
        id: "forms.option",
        message:
          "{key, select, min {Min} max {Max} pattern {Pattern} region {Region} maxScale {Decimals} maxIntegerDigits {Whole digits} rule {Rule} debounceMs {Delay (ms)} message {Message} other {Option}}",
        values: { key },
        comment: "An option of a validator, e.g. the least length or the regular expression.",
      }),
    removeValidator: i18n._({
      id: "forms.removeValidator",
      message: "Remove this check",
      comment: "Button in a validator's options.",
    }),
    preview: i18n._({ id: "forms.preview", message: "Preview", comment: "Which state of the form the canvas draws." }),
    mode: (mode: string) =>
      i18n._({
        id: "forms.mode",
        message:
          "{mode, select, pristine {Pristine} errors {Errors} valid {Valid} submitting {Submitting} other {State}}",
        values: { mode },
        comment:
          "A state a form is previewed in: untouched, showing its validation errors, filled with valid values, or being sent.",
      }),
    formField: i18n._({
      id: "forms.formField",
      message: "Form field",
      comment: "Heading of the panel that binds an input to a form field.",
    }),
    bindTo: i18n._({
      id: "forms.bindTo",
      message: "Bind to a field",
      comment: "Label of the select of form fields an input can be bound to.",
    }),
    bound: (form: string, field: string) =>
      i18n._({
        id: "forms.bound",
        message: "Bound to {form}.{field}",
        values: { form, field },
        comment: "An input bound to a form field: its value reads it, a change writes it and its error shows.",
      }),
    noForms: i18n._({
      id: "forms.noForms",
      message: "The page has no form yet. Add one under Page › Forms.",
      comment: "Shown in the form field panel when the page declares no form.",
    }),
  };
}

/** The validators' messages at design time (the runtime's built-in strings until P8 localises them, ADR-0047). */
export function validatorTexts(i18n: I18n): ValidatorTexts {
  return {
    required: i18n._({ id: "forms.v.required", message: "This field is required", comment: "Validator message." }),
    length: (min, max) =>
      i18n._({
        id: "forms.v.length",
        message: "Use {min} to {max} characters",
        values: { min: min ?? 0, max: max ?? "∞" },
        comment: "Validator message: the text's length is out of range.",
      }),
    range: (min, max) =>
      i18n._({
        id: "forms.v.range",
        message: "Enter a value from {min} to {max}",
        values: { min: min ?? "−∞", max: max ?? "∞" },
        comment: "Validator message: the number is out of range.",
      }),
    invalid: i18n._({
      id: "forms.v.invalid",
      message: "Check this value",
      comment: "Validator message: a pattern does not match.",
    }),
    email: i18n._({ id: "forms.v.email", message: "Enter a valid email address", comment: "Validator message." }),
    phone: i18n._({ id: "forms.v.phone", message: "Enter a valid phone number", comment: "Validator message." }),
    iban: i18n._({ id: "forms.v.iban", message: "Enter a valid IBAN", comment: "Validator message." }),
  };
}

export type FormsMessages = ReturnType<typeof formsMessages>;
