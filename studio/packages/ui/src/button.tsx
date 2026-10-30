// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ComponentPropsWithoutRef, ReactElement } from "react";
import { Button as ShadcnButton } from "./components/button";
import { Spinner } from "./components/spinner";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
export type ButtonSize = "sm" | "md" | "lg";

/** Studio's variants as shadcn's: the primary action, an outlined secondary one, a quiet one and a destructive one. */
const variants = { primary: "default", secondary: "outline", ghost: "ghost", danger: "destructive" } as const;
const sizes = { sm: "sm", md: "default", lg: "lg" } as const;

export type ButtonProps = Omit<ComponentPropsWithoutRef<typeof ShadcnButton>, "variant" | "size"> & {
  readonly variant?: ButtonVariant;
  readonly size?: ButtonSize;
  /** Busy state: the button stays focusable, ignores presses and announces `aria-busy`. */
  readonly loading?: boolean;
  /** Visually hidden text announced while loading, from the message catalogue. */
  readonly loadingLabel?: string;
};

/** shadcn's Button (ADR-0103). Disabled and loading buttons stay focusable so keyboard users keep their place. */
export function Button({
  variant = "primary",
  size = "md",
  loading = false,
  loadingLabel,
  disabled,
  children,
  ...rest
}: ButtonProps): ReactElement {
  return (
    <ShadcnButton
      {...rest}
      variant={variants[variant]}
      size={sizes[size]}
      disabled={disabled === true || loading}
      focusableWhenDisabled
      aria-busy={loading || undefined}
      data-variant={variant}
    >
      {loading && <Spinner data-icon="inline-start" aria-hidden="true" />}
      {loading && loadingLabel !== undefined && <span className="sr-only">{loadingLabel}</span>}
      {children}
    </ShadcnButton>
  );
}
