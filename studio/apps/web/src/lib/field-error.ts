// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** What a form field's state says about its errors (TanStack Form's field meta). */
export interface FieldErrorState {
  readonly isBlurred: boolean;
  readonly errors: readonly unknown[];
}

/**
 * The error to show under a field, or `undefined` (STU-011). A field is left alone while it is
 * first typed into: its error appears once the person leaves the field or tries to submit, and
 * from then on it follows every keystroke, so it goes away as soon as the value is valid.
 */
export function shownError(meta: FieldErrorState, submitted: boolean): string | undefined {
  if (!meta.isBlurred && !submitted) {
    return undefined;
  }
  const first = meta.errors[0];
  if (typeof first === "string") {
    return first;
  }
  return typeof first === "object" && first !== null && "message" in first ? String(first.message) : undefined;
}
