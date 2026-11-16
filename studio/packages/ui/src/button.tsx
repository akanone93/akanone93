// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ComponentPropsWithoutRef, ReactElement } from "react";
import { Button as ShadcnButton } from "./components/button";
import { Spinner } from "./components/spinner";
import { cx } from "./cx.ts";

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

/**
 * Base UI marks a disabled button that stays focusable with `aria-disabled`, not the `disabled` attribute, so shadcn's
 * `disabled:` styles never match it: the dimmed look is keyed to `aria-disabled` instead, or a disabled action would
 * look as pressable as an enabled one.
 */
const disabledLook = "aria-disabled:opacity-50 aria-disabled:cursor-not-allowed";

/** shadcn's Button (ADR-0103). Disabled and loading buttons stay focusable so keyboard users keep their place. */
export function Button({
  variant = "primary",
  size = "md",
  loading = false,
  loadingLabel,
  disabled,
  className,
  children,
  ...rest
}: ButtonProps): ReactElement {
  return (
    <ShadcnButton
      {...rest}
      className={
        typeof className === "function" ? (state) => cx(disabledLook, className(state)) : cx(disabledLook, className)
      }
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
