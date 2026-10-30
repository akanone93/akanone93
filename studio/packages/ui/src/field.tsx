// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Field as BaseField } from "@base-ui/react/field";
import type { ComponentPropsWithoutRef, ReactElement, ReactNode } from "react";
import { FieldDescription, FieldError, FieldLabel, Field as ShadcnField } from "./components/field";
import { Input as ShadcnInput } from "./components/input";
import { InputGroup, InputGroupAddon, InputGroupInput } from "./components/input-group";
import { Textarea as ShadcnTextarea } from "./components/textarea";
import { Icon } from "./icon.tsx";
import type { IconName } from "./icons.ts";

export interface FieldProps {
  readonly label: ReactNode;
  readonly description?: ReactNode;
  /** When set the field is invalid and the message is linked to the control by `aria-describedby`. */
  readonly error?: ReactNode;
  readonly children: ReactNode;
}

/**
 * shadcn's Field (ADR-0103), with Base UI's Field underneath so the label, description and error are wired to the
 * control without ids (STU-011).
 */
export function Field({ label, description, error, children }: FieldProps): ReactElement {
  const invalid = error !== undefined;
  return (
    <BaseField.Root invalid={invalid} render={<ShadcnField {...(invalid && { "data-invalid": true })} />}>
      <BaseField.Label render={<FieldLabel />}>{label}</BaseField.Label>
      {children}
      {description !== undefined && (
        <BaseField.Description render={<FieldDescription />}>{description}</BaseField.Description>
      )}
      {invalid && (
        <BaseField.Error match render={<FieldError />}>
          {error}
        </BaseField.Error>
      )}
    </BaseField.Root>
  );
}

/** `lg` is the taller control of the public pages. */
export type InputSize = "md" | "lg";

export type InputProps = Omit<ComponentPropsWithoutRef<"input">, "className" | "size"> & {
  readonly size?: InputSize;
  /** An icon at the start of the field, decorative. */
  readonly leading?: IconName;
  /** A control at the end of the field, such as a button that shows a password. */
  readonly trailing?: ReactNode;
};

/** shadcn's Input, or its InputGroup when the field carries an icon or a trailing control (ADR-0103). */
export function Input({ size = "md", leading, trailing, ...props }: InputProps): ReactElement {
  const height = size === "lg" ? "h-9" : undefined;
  if (leading === undefined && trailing === undefined) {
    return <ShadcnInput {...props} {...(height !== undefined && { className: height })} />;
  }
  return (
    <InputGroup {...(height !== undefined && { className: height })}>
      <InputGroupInput {...props} />
      {leading !== undefined && (
        <InputGroupAddon>
          <Icon name={leading} />
        </InputGroupAddon>
      )}
      {trailing !== undefined && <InputGroupAddon align="inline-end">{trailing}</InputGroupAddon>}
    </InputGroup>
  );
}

export type TextareaProps = Omit<ComponentPropsWithoutRef<"textarea">, "className">;

/** shadcn's Textarea, registered with the surrounding Field. */
export function Textarea(props: TextareaProps): ReactElement {
  return <BaseField.Control render={<ShadcnTextarea {...props} />} />;
}
