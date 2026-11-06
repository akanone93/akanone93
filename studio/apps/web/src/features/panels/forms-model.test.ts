// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { designScopes, formKey } from "../plugin-canvas/design-scope.ts";
import {
  addFieldRecipe,
  addFormRecipe,
  addValidatorRecipe,
  bindFieldRecipe,
  boundField,
  formState,
  formsOf,
  ibanValid,
  removeValidatorRecipe,
  sampleValue,
  setValidatorRecipe,
  type ValidatorTexts,
  validateField,
} from "./forms-model.ts";

const t: ValidatorTexts = {
  required: "Required",
  length: (a, b) => `len ${a}-${b}`,
  range: (a, b) => `range ${a}-${b}`,
  invalid: "Invalid",
  email: "Email",
  phone: "Phone",
  iban: "IBAN",
};
const page = {
  kind: "page",
  id: "p",
  forms: [
    {
      id: "f",
      name: "signup",
      fields: [
        {
          name: "name",
          type: "string",
          initial: "",
          validators: [
            { kind: "required", message: "Enter your name" },
            { kind: "length", min: 2, max: 20 },
          ],
        },
        { name: "email", type: "string", initial: "ada@", validators: [{ kind: "email" }] },
        { name: "age", type: "int?", initial: 12, validators: [{ kind: "range", min: 18, max: 130 }] },
        { name: "iban", type: "string", validators: [{ kind: "iban" }] },
      ],
    },
  ],
};

describe("PC-26 forms at design time [STA-020]", () => {
  test("built-in checks give their declared message, else the built-in one; valid values pass", () => {
    const [form] = formsOf(page);
    const [name, email, age, iban] = form?.fields ?? [];
    expect(name && validateField(name, "", t)).toBe("Enter your name");
    expect(name && validateField(name, "A", t)).toBe("len 2-20");
    expect(email && validateField(email, "ada@", t)).toBe("Email");
    expect(age && validateField(age, 12, t)).toBe("range 18-130");
    expect(ibanValid("GB82 WEST 1234 5698 7654 32")).toBe(true);
    expect(ibanValid("GB82WEST12345698765431")).toBe(false);
    for (const f of form?.fields ?? []) {
      expect(validateField(f, sampleValue(f), t)).toBeUndefined();
    }
    expect(iban && sampleValue(iban)).toBe("GB82WEST12345698765432");
  });

  test("a form's state per preview mode lands in the page scope under its name", () => {
    const [form] = formsOf(page);
    if (form === undefined) {
      throw new Error("no form");
    }
    expect(formState(form, "pristine", t)).toMatchObject({
      errors: {},
      status: "idle",
      valid: true,
      touched: { name: false },
    });
    expect(formState(form, "errors", t)).toMatchObject({
      errors: { name: "Enter your name", email: "Email", age: "range 18-130" },
      valid: false,
    });
    expect(formState(form, "submitting", t)).toMatchObject({
      status: "submitting",
      values: { email: "ada@example.com" },
    });
    const doc = { kind: "page", content: new Uint8Array(), parsed: page };
    const scope = designScopes([doc], { modes: new Map([[formKey("p", "signup"), "errors"]]), text: t }).get("p");
    expect(scope?.page["signup"]).toMatchObject({ errors: { name: "Enter your name" } });
  });

  test("forms, fields and checks are edited in the page; an input binds to a field in one step", () => {
    const draft: Record<string, unknown> = { kind: "page" };
    addFormRecipe("id1", "login")(draft);
    addFieldRecipe(0, "password")(draft);
    addValidatorRecipe(0, 1, "length")(draft);
    setValidatorRecipe(0, 1, 0, "min", 8)(draft);
    expect(formsOf(draft)[0]?.fields[1]?.validators).toEqual([{ kind: "length", min: 8, max: 100 }]);
    removeValidatorRecipe(0, 1, 0)(draft);
    expect(formsOf(draft)[0]?.fields[1]?.validators).toEqual([]);
    const root = {
      id: "r",
      type: "Column",
      children: [{ id: "tf", type: "TextField", props: { decoration: { labelText: "Name" } } }],
    };
    bindFieldRecipe("tf", "signup", "name")(root);
    const tf = root.children[0] as Record<string, unknown>;
    expect(tf["props"]).toEqual({
      value: { $expr: "page.signup.values.name" },
      decoration: { labelText: "Name", errorText: { $expr: "page.signup.errors.name" } },
    });
    expect(JSON.stringify(tf["events"])).toContain("page.signup.touched.name");
    expect(boundField(tf)).toEqual({ form: "signup", field: "name" });
  });
});
