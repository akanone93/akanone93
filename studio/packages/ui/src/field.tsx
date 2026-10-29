// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Field as BaseField } from "@base-ui/react/field";
import type { ComponentPropsWithoutRef, ReactElement, ReactNode } from "react";
import { cx, focusRing } from "./cx.ts";

export interface FieldProps {
  readonly label: ReactNode;
  readonly description?: ReactNode;
  /** When set the field is invalid and the message is linked to the control by `aria-describedby`. */
  readonly error?: ReactNode;
  readonly children: ReactNode;
}

/** Label, control, description and error wired together by Base UI Field (STU-011). */
export function Field({ label, description, error, children }: FieldProps): ReactElement {
  return (
    <BaseField.Root invalid={error !== undefined} className="flex flex-col gap-1">
      <BaseField.Label className="text-label font-medium text-primary">{label}</BaseField.Label>
      {children}
      {description !== undefined && (
        <BaseField.Description className="text-caption text-tertiary">{description}</BaseField.Description>
      )}
      {error !== undefined && (
        <BaseField.Error match className="text-caption text-danger">
          {error}
        </BaseField.Error>
      )}
    </BaseField.Root>
  );
}

const controlClass = cx(
  "w-full text-primary transition duration-fast ease-standard",
  "border border-hairline hover:border-strong",
  "data-invalid:border-danger-fill",
  "disabled:opacity-50",
  focusRing,
);

export type InputSize = "md" | "lg";

const inputSizeClass: Readonly<Record<InputSize, string>> = {
  md: "h-(--density-field) rounded-sm bg-raised px-3 text-body",
  // The public pages (U11.2): a 48 px field with 16 px text, a soft fill and a halo on focus.
  lg: "h-12 rounded-lg bg-raised/70 px-4 text-control focus-visible:ring-4 focus-visible:ring-accent-halo",
};

export type InputProps = Omit<ComponentPropsWithoutRef<"input">, "className" | "size"> & {
  readonly size?: InputSize;
};

export function Input({ size = "md", ...props }: InputProps): ReactElement {
  return <BaseField.Control {...props} className={cx(controlClass, inputSizeClass[size])} />;
}

export type TextareaProps = Omit<ComponentPropsWithoutRef<"textarea">, "className">;

export function Textarea(props: TextareaProps): ReactElement {
  return (
    <BaseField.Control
      render={<textarea {...props} />}
      className={cx(controlClass, "rounded-sm bg-raised px-3 py-2 text-body")}
    />
  );
}
