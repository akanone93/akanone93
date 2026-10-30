// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { shownError } from "./field-error.ts";

const invalid = [{ message: "Enter a valid email address." }];

describe("[STU-011] field errors wait until the field is left or the form is submitted", () => {
  test("no error while the field is first typed into", () => {
    expect(shownError({ isBlurred: false, errors: invalid }, false)).toBeUndefined();
  });

  test("the error shows once the field was left, or the form submitted", () => {
    expect(shownError({ isBlurred: true, errors: invalid }, false)).toBe("Enter a valid email address.");
    expect(shownError({ isBlurred: false, errors: invalid }, true)).toBe("Enter a valid email address.");
  });

  test("the error goes away as soon as the value is valid", () => {
    expect(shownError({ isBlurred: true, errors: [] }, true)).toBeUndefined();
  });

  test("a plain string error is shown as it is", () => {
    expect(shownError({ isBlurred: true, errors: ["Too short."] }, false)).toBe("Too short.");
  });
});
