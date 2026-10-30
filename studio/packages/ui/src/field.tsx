// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Field as BaseField } from "@base-ui/react/field";
import type { ComponentPropsWithoutRef, ReactElement, ReactNode } from "react";
import { cx, focusRing } from "./cx.ts";
import { Icon } from "./icon.tsx";
import type { IconName } from "./icons.gen.ts";

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
  "w-full border text-primary transition duration-fast ease-standard",
  "data-invalid:border-danger-fill",
  "disabled:opacity-50",
);

export type InputSize = "md" | "lg";

const inputSizeClass: Readonly<Record<InputSize, string>> = {
  md: cx("h-(--density-field) rounded-sm border-hairline bg-raised px-3 text-body hover:border-strong", focusRing),
  // The public pages (U11.2): a 48 px field with 16 px text on a soft fill, no line until it is
  // hovered; on focus it lifts to the raised surface with an accent edge and a halo.
  lg: cx(
    "h-12 rounded-lg border-transparent bg-canvas px-4 text-control placeholder:text-tertiary",
    "hover:border-strong focus-visible:border-accent focus-visible:bg-raised",
    "focus-visible:outline-hidden focus-visible:ring-4 focus-visible:ring-accent-halo",
  ),
};

/** Room left at the start or end of a control for its icon or trailing button. */
const leadingRoom: Readonly<Record<InputSize, string>> = { md: "ps-8", lg: "ps-12" };
const trailingRoom: Readonly<Record<InputSize, string>> = { md: "pe-8", lg: "pe-12" };

export type InputProps = Omit<ComponentPropsWithoutRef<"input">, "className" | "size"> & {
  readonly size?: InputSize;
  /** An icon at the start of the field, decorative. */
  readonly leading?: IconName;
  /** A control at the end of the field, such as a button that shows a password. */
  readonly trailing?: ReactNode;
};

export function Input({ size = "md", leading, trailing, ...props }: InputProps): ReactElement {
  const control = (
    <BaseField.Control
      {...props}
      className={cx(
        controlClass,
        inputSizeClass[size],
        leading !== undefined && leadingRoom[size],
        trailing !== undefined && trailingRoom[size],
      )}
    />
  );
  if (leading === undefined && trailing === undefined) {
    return control;
  }
  return (
    <span className="relative flex items-center">
      {leading !== undefined && (
        <span className={cx("pointer-events-none absolute text-tertiary", size === "lg" ? "start-4" : "start-2")}>
          <Icon name={leading} size="tab" />
        </span>
      )}
      {control}
      {trailing !== undefined && <span className="absolute end-2 flex items-center">{trailing}</span>}
    </span>
  );
}

export type TextareaProps = Omit<ComponentPropsWithoutRef<"textarea">, "className">;

export function Textarea(props: TextareaProps): ReactElement {
  return (
    <BaseField.Control
      render={<textarea {...props} />}
      className={cx(
        controlClass,
        "rounded-sm border-hairline bg-raised px-3 py-2 text-body hover:border-strong",
        focusRing,
      )}
    />
  );
}
